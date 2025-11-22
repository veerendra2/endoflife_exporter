package endoflife

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	doRequest(ctx context.Context, requestUrl string) ([]byte, error)
	GetProductDetails(ctx context.Context, productName string) ([]ReleaseDetails, error)
	GetRelease(ctx context.Context, productName string, cycleName string) (ReleaseDetails, error)
}

// doRequest does HTTP request to given requestUrl and returns response body
func (c *client) doRequest(ctx context.Context, requestUrl string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("Error while closing the response body", slog.Any("err", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-OK status: %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// GetRelease retrieves details for a specific release cycle of a product.
// Endpoint: GET /products/{productName}/releases/{cycleName}
func (c *client) GetRelease(ctx context.Context, productName string, cycleName string) (ReleaseDetails, error) {
	requestUrl := *c.baseUrl
	releaseDetails := ReleaseDetails{}
	productRelease := ProductReleaseResponse{}

	requestUrl.Path = path.Join(requestUrl.Path, "products", productName, "releases", cycleName)

	body, err := c.doRequest(ctx, requestUrl.String())
	if err != nil {
		return releaseDetails, err
	}

	if err := json.Unmarshal(body, &productRelease); err != nil {
		return releaseDetails, fmt.Errorf("failed to decode API response: %w", err)
	}

	releaseDetails = getReleaseDetails(productRelease.Result)

	return releaseDetails, nil
}

// GetProductDetails retrieves all release cycles for a given product.
// Endpoint: GET /products/{productName}
func (c *client) GetProductDetails(ctx context.Context, productName string) ([]ReleaseDetails, error) {
	requestUrl := *c.baseUrl
	releaseDetails := []ReleaseDetails{}
	product := ProductResponse{}

	requestUrl.Path = path.Join(requestUrl.Path, "products", productName)

	body, err := c.doRequest(ctx, requestUrl.String())
	if err != nil {
		return releaseDetails, err
	}

	if err := json.Unmarshal(body, &product); err != nil {
		return releaseDetails, fmt.Errorf("failed to decode API response: %w", err)
	}

	for _, productRelease := range product.Result.Releases {
		releaseDetails = append(releaseDetails, getReleaseDetails(productRelease))
	}

	return releaseDetails, nil
}

// getReleaseDetails converts a ProductRelease from the API response into a ReleaseDetails struct.
func getReleaseDetails(productRelease ProductRelease) ReleaseDetails {
	latestVersion := "N/A"
	latestVersionDate := time.Unix(0, 0)
	eolFrom := time.Unix(2524608000, 0) // Default is 2050-01-01
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
