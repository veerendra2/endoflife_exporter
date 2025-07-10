package endoflife

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"time"
)

const EndOfLifeBaseURL = "https://endoflife.date/api/v1"

type ReleaseDetails struct {
	EOLFrom           time.Time
	IsEol             bool
	IsLts             bool
	IsMaintained      bool
	LatestVersion     string
	LatestVersionDate time.Time
	ReleaseCycleDate  time.Time
	ReleaseCycleName  string
}

type client struct {
	baseUrl    *url.URL
	httpClient http.Client
}

type Client interface {
	doRequest(ctx context.Context, requestUrl string, target any) error
	GetProductDetails(ctx context.Context, productName string) ([]ReleaseDetails, error)
	GetRelease(ctx context.Context, productName string, cycleName string) (ReleaseDetails, error)
}

func (c *client) doRequest(ctx context.Context, requestUrl string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return err
	}

	req.Header.Set("accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("Error while closing the reponse body", slog.Any("err", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned non-OK status: %d %s", resp.StatusCode, resp.Status)
	}

	if err = json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode API response: %w", err)
	}
	return nil
}

func (c *client) GetRelease(ctx context.Context, productName string, cycleName string) (ReleaseDetails, error) {
	requestUrl := *c.baseUrl
	releaseDetails := ReleaseDetails{}
	productRelease := ProductReleaseResponse{}

	// https://endoflife.date/api/v1/products/{productName}/releases/{cycleName}
	requestUrl.Path = path.Join(requestUrl.Path, "products", productName, "releases", cycleName)

	if err := c.doRequest(ctx, requestUrl.String(), &productRelease); err != nil {
		return releaseDetails, err
	}

	releaseDetails = getReleaseDetails(productRelease.Result)

	return releaseDetails, nil
}

func (c *client) GetProductDetails(ctx context.Context, productName string) ([]ReleaseDetails, error) {
	requestUrl := *c.baseUrl
	releaseDetails := []ReleaseDetails{}
	product := ProductResponse{}

	// https://endoflife.date/api/v1/products/{productName}
	requestUrl.Path = path.Join(requestUrl.Path, "products", productName)

	if err := c.doRequest(ctx, requestUrl.String(), &product); err != nil {
		return releaseDetails, err
	}

	for _, productRelease := range product.Result.Releases {
		releaseDetails = append(releaseDetails, getReleaseDetails(productRelease))
	}

	return releaseDetails, nil
}

func getReleaseDetails(productRelease ProductRelease) ReleaseDetails {
	latestVersion := "N/A"
	latestVersionDate := time.Unix(0, 0)
	eolFrom := time.Unix(0, 0)
	releaseCycleDate := time.Unix(0, 0)

	if latest, err := productRelease.Latest.AsProductVersion(); err == nil {
		latestVersion = latest.Name
		if parsedDate, err := time.Parse("2006-01-02", latest.Date.String()); err == nil {
			latestVersionDate = parsedDate
		}
	}

	if productRelease.EolFrom != nil {
		if parsedDate, err := time.Parse("2006-01-02", productRelease.EolFrom.String()); err == nil {
			eolFrom = parsedDate
		}
	}

	if parsedDate, err := time.Parse("2006-01-02", productRelease.ReleaseDate.String()); err == nil {
		releaseCycleDate = parsedDate
	}

	return ReleaseDetails{
		EOLFrom:           eolFrom,
		IsLts:             productRelease.IsLts,
		IsEol:             productRelease.IsEol,
		IsMaintained:      productRelease.IsMaintained,
		LatestVersion:     latestVersion,
		LatestVersionDate: latestVersionDate,
		ReleaseCycleDate:  releaseCycleDate,
		ReleaseCycleName:  productRelease.Name,
	}
}

func NewClient() (Client, error) {
	baseUrl, err := url.Parse(EndOfLifeBaseURL)
	if err != nil {
		return nil, err
	}

	return &client{
		baseUrl:    baseUrl,
		httpClient: http.Client{},
	}, nil
}
