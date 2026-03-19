// Package services provides business logic and orchestration for the application.
package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/sync/singleflight"
)

// CartLineInput handles a single product action
type CartLineInput struct {
	MerchandiseID string `json:"merchandiseId"`
	Quantity      int    `json:"quantity"`
}

// AttributeInput represents a custom key-value pair attached to the cart.
type AttributeInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ShopifyService handles communication with Shopify APIs and webhook verification.
type ShopifyService struct {
	logger          *logging.ChanneledLogger
	tenantManager   *tenant.Manager
	resourceService *ResourceService
	requestGroup    singleflight.Group
}

// NewShopifyService creates a new Shopify service instance.
func NewShopifyService(logger *logging.ChanneledLogger, tenantManager *tenant.Manager, resourceService *ResourceService) *ShopifyService {
	return &ShopifyService{
		logger:          logger,
		tenantManager:   tenantManager,
		resourceService: resourceService,
	}
}

// VerifySignature validates the Shopify webhook HMAC signature.
func (s *ShopifyService) VerifySignature(tenantCtx *tenant.Context, body []byte, signature string) bool {
	secret := tenantCtx.Config.ShopifyAPISecret
	if secret == "" {
		s.logger.System().Warn("Shopify API Secret not configured for tenant", "tenantId", tenantCtx.TenantID)
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := base64.StdEncoding.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ParseWebhook converts a raw Shopify webhook payload into a ResourceNode.
func (s *ShopifyService) ParseWebhook(body []byte) (*content.ResourceNode, error) {
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook body: %w", err)
	}

	var idStr string
	if v, ok := rawData["id"].(float64); ok {
		idStr = fmt.Sprintf("%.0f", v)
	} else if v, ok := rawData["id"].(string); ok {
		idStr = v
	}

	if idStr == "" {
		return nil, fmt.Errorf("missing 'id' in webhook payload")
	}

	gid := idStr
	if !strings.HasPrefix(idStr, "gid://") {
		gid = fmt.Sprintf("gid://shopify/Product/%s", idStr)
	}

	title, _ := rawData["title"].(string)
	handle, _ := rawData["handle"].(string)
	description, _ := rawData["body_html"].(string)

	oneLiner := ""
	if description != "" {
		oneLiner = bluemonday.StrictPolicy().Sanitize(description)
		if len(oneLiner) > 255 {
			oneLiner = oneLiner[:252] + "..."
		}
	}

	optionsPayload := make(map[string]any)
	optionsPayload["gid"] = gid
	jsonData, _ := json.Marshal(rawData)
	optionsPayload["shopifyData"] = string(jsonData)

	slug := ""
	if handle != "" {
		slug = fmt.Sprintf("product-%s", handle)
	}

	return &content.ResourceNode{
		Title:          title,
		Slug:           slug,
		OneLiner:       oneLiner,
		NodeType:       "Resource",
		OptionsPayload: optionsPayload,
	}, nil
}

// FetchProducts queries the Shopify Storefront API via GraphQL.
// It uses ephemeral caching and singleflight to prevent cross-search data leakage and memory exhaustion.
func (s *ShopifyService) FetchProducts(tenantCtx *tenant.Context, queryStr string, cursor *string) (map[string]any, error) {
	cursorStr := ""
	if cursor != nil {
		cursorStr = *cursor
	}
	cacheKey := fmt.Sprintf("shopify_fetch_%s_q:%s_c:%s", tenantCtx.TenantID, queryStr, cursorStr)

	if cached, found := tenantCtx.CacheManager.GetGeneric(tenantCtx.TenantID, cacheKey); found {
		if result, ok := cached.(map[string]any); ok {
			return result, nil
		}
	}

	v, err, _ := s.requestGroup.Do(cacheKey, func() (any, error) {
		token := tenantCtx.Config.ShopifyStorefrontToken
		domain := tenantCtx.Config.ShopifyStoreDomain
		apiVersion := tenantCtx.Config.ShopifyAPIVersion

		if token == "" || domain == "" {
			return nil, fmt.Errorf("shopify credentials missing for tenant %s", tenantCtx.TenantID)
		}
		if apiVersion == "" {
			return nil, fmt.Errorf("shopify api version not configured for tenant %s", tenantCtx.TenantID)
		}

		cleanDomain := strings.TrimSuffix(domain, "/")
		if !strings.HasPrefix(cleanDomain, "http") {
			cleanDomain = "https://" + cleanDomain
		}
		url := fmt.Sprintf("%s/api/%s/graphql.json", cleanDomain, apiVersion)

		queryTemplate := `
        query ($cursor: String, $query: String) {
          products(first: 25, after: $cursor, query: $query) {
            pageInfo {
              hasNextPage
              endCursor
            }
            edges {
              node {
                id
                title
                handle
                description
                vendor
                tags
                options {
                  name
                  values
                }
                images(first: 20) {
                  edges {
                    node {
                      url
                      altText
                    }
                  }
                }
                variants(first: 250) {
                  edges {
                    node {
                      id
                      title
                      price {
                        amount
                        currencyCode
                      }
                      image {
                        url
                      }
                      compareAtPrice {
                        amount
                        currencyCode
                      }
                      sku
                      availableForSale
                      requiresShipping
                      selectedOptions {
                        name
                        value
                      }
                    }
                  }
                }
              }
            }
          }
        }
    `

		variables := map[string]any{}
		if queryStr != "" {
			variables["query"] = "title:" + queryStr + "*"
		}
		if cursor != nil {
			variables["cursor"] = *cursor
		}

		reqBody := map[string]any{
			"query":     queryTemplate,
			"variables": variables,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create shopify request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Shopify-Storefront-Private-Token", token)

		client := &http.Client{Timeout: config.ShopifyRequestTimeout}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("shopify request failed: %w", err)
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				s.logger.System().Warn("Failed to close Shopify response body", "error", closeErr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("shopify api error: %s %s", resp.Status, string(b))
		}

		var result struct {
			Data struct {
				Products struct {
					PageInfo map[string]any `json:"pageInfo"`
					Edges    []any          `json:"edges"`
				} `json:"products"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode shopify response: %w", err)
		}

		if len(result.Errors) > 0 {
			return nil, fmt.Errorf("graphql errors: %v", result.Errors)
		}

		finalProducts := make([]map[string]any, 0, len(result.Data.Products.Edges))
		for _, e := range result.Data.Products.Edges {
			edge, ok := e.(map[string]any)
			if !ok {
				continue
			}
			node, ok := edge["node"].(map[string]any)
			if !ok {
				continue
			}

			if imgs, ok := node["images"].(map[string]any); ok {
				if iEdges, ok := imgs["edges"].([]any); ok {
					flatImages := make([]map[string]any, 0)
					for _, ie := range iEdges {
						if iEdge, ok := ie.(map[string]any); ok {
							if iNode, ok := iEdge["node"].(map[string]any); ok {
								flatImages = append(flatImages, iNode)
							}
						}
					}
					node["images"] = flatImages
				}
			}

			if vars, ok := node["variants"].(map[string]any); ok {
				if vEdges, ok := vars["edges"].([]any); ok {
					flatVariants := make([]map[string]any, 0)
					for _, ve := range vEdges {
						if vEdge, ok := ve.(map[string]any); ok {
							if vNode, ok := vEdge["node"].(map[string]any); ok {
								flatVariants = append(flatVariants, vNode)
							}
						}
					}
					node["variants"] = flatVariants
				}
			}

			finalProducts = append(finalProducts, node)
		}

		responseEnvelope := map[string]any{
			"products": finalProducts,
			"pageInfo": result.Data.Products.PageInfo,
		}

		tenantCtx.CacheManager.SetGenericWithTTL(tenantCtx.TenantID, cacheKey, responseEnvelope, 60*time.Second)
		return responseEnvelope, nil
	})

	if err != nil {
		return nil, err
	}

	return v.(map[string]any), nil
}

// ReconcileAll performs targeted background synchronization of tracked Shopify products.
// It chunks local GIDs and uses the nodes query to respect pagination limits.
func (s *ShopifyService) ReconcileAll(tenantCtx *tenant.Context) (int, int, int, error) {
	localProducts, _ := s.resourceService.GetByCategory(tenantCtx, "product")
	localServices, _ := s.resourceService.GetByCategory(tenantCtx, "service")

	var allGIDs []string
	gidToResource := make(map[string]*content.ResourceNode)

	for _, local := range append(localProducts, localServices...) {
		if gid, ok := local.OptionsPayload["gid"].(string); ok && gid != "" {
			allGIDs = append(allGIDs, gid)
			gidToResource[gid] = local
		}
	}

	if len(allGIDs) == 0 {
		return 0, 0, 0, nil
	}

	token := tenantCtx.Config.ShopifyStorefrontToken
	domain := tenantCtx.Config.ShopifyStoreDomain
	apiVersion := tenantCtx.Config.ShopifyAPIVersion

	if token == "" || domain == "" || apiVersion == "" {
		return 0, 0, 0, fmt.Errorf("shopify credentials or api version missing for tenant %s", tenantCtx.TenantID)
	}

	cleanDomain := strings.TrimSuffix(domain, "/")
	if !strings.HasPrefix(cleanDomain, "http") {
		cleanDomain = "https://" + cleanDomain
	}
	url := fmt.Sprintf("%s/api/%s/graphql.json", cleanDomain, apiVersion)

	queryTemplate := `
        query ($ids: [ID!]!) {
          nodes(ids: $ids) {
            ... on Product {
              id
              title
              handle
              description
              vendor
              tags
              options {
                name
                values
              }
              images(first: 20) {
                edges {
                  node {
                    url
                    altText
                  }
                }
              }
              variants(first: 250) {
                edges {
                  node {
                    id
                    title
                    price {
                      amount
                      currencyCode
                    }
                    image {
                      url
                    }
                    compareAtPrice {
                      amount
                      currencyCode
                    }
                    sku
                    availableForSale
                    requiresShipping
                    selectedOptions {
                      name
                      value
                    }
                  }
                }
              }
            }
          }
        }
    `

	client := &http.Client{Timeout: config.ShopifyRequestTimeout}
	batchSize := 250
	totalProcessed := len(allGIDs)
	reconciledCount := 0
	deletedCount := 0
	pCleaner := bluemonday.StrictPolicy()

	for i := 0; i < len(allGIDs); i += batchSize {
		end := i + batchSize
		if end > len(allGIDs) {
			end = len(allGIDs)
		}
		chunkIDs := allGIDs[i:end]

		reqBody := map[string]any{
			"query": queryTemplate,
			"variables": map[string]any{
				"ids": chunkIDs,
			},
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		if err != nil {
			s.logger.System().Error("Failed to create shopify nodes request", "error", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Shopify-Storefront-Private-Token", token)

		resp, err := client.Do(req)
		if err != nil {
			s.logger.System().Error("Shopify nodes request failed", "error", err)
			continue
		}

		var result struct {
			Data struct {
				Nodes []any `json:"nodes"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close()
			s.logger.System().Error("Failed to decode shopify nodes response", "error", err)
			continue
		}
		_ = resp.Body.Close()

		if len(result.Errors) > 0 {
			s.logger.System().Error("Graphql errors in nodes fetch", "errors", result.Errors)
			continue
		}

		for idx, nodeRaw := range result.Data.Nodes {
			requestedGID := chunkIDs[idx]

			if nodeRaw == nil {
				op, err := s.resourceService.SyncShopifyDeletion(tenantCtx, requestedGID)
				if err != nil {
					s.logger.System().Error("Failed to prune orphaned Shopify resource", "error", err, "gid", requestedGID)
				} else if op == "deleted" {
					deletedCount++
				}
				continue
			}

			node, ok := nodeRaw.(map[string]any)
			if !ok {
				continue
			}

			if imgs, ok := node["images"].(map[string]any); ok {
				if iEdges, ok := imgs["edges"].([]any); ok {
					flatImages := make([]map[string]any, 0)
					for _, ie := range iEdges {
						if iEdge, ok := ie.(map[string]any); ok {
							if iNode, ok := iEdge["node"].(map[string]any); ok {
								flatImages = append(flatImages, iNode)
							}
						}
					}
					node["images"] = flatImages
				}
			}

			if vars, ok := node["variants"].(map[string]any); ok {
				if vEdges, ok := vars["edges"].([]any); ok {
					flatVariants := make([]map[string]any, 0)
					for _, ve := range vEdges {
						if vEdge, ok := ve.(map[string]any); ok {
							if vNode, ok := vEdge["node"].(map[string]any); ok {
								flatVariants = append(flatVariants, vNode)
							}
						}
					}
					node["variants"] = flatVariants
				}
			}

			id, _ := node["id"].(string)
			handle, _ := node["handle"].(string)
			title, _ := node["title"].(string)
			description, _ := node["description"].(string)

			oneLiner := pCleaner.Sanitize(description)
			if len(oneLiner) > 255 {
				oneLiner = oneLiner[:252] + "..."
			}

			optionsPayload := make(map[string]any)
			optionsPayload["gid"] = id
			jsonData, _ := json.Marshal(node)
			optionsPayload["shopifyData"] = string(jsonData)

			var mainImageURL string
			if rawImages, ok := node["images"].([]any); ok && len(rawImages) > 0 {
				if firstImg, ok := rawImages[0].(map[string]any); ok {
					mainImageURL, _ = firstImg["url"].(string)
				}
			} else if rawImages, ok := node["images"].([]map[string]any); ok && len(rawImages) > 0 {
				mainImageURL, _ = rawImages[0]["url"].(string)
			}

			shopifyImageMap := make(map[string]any)
			var rawVariants []any
			if v, ok := node["variants"].([]any); ok {
				rawVariants = v
			} else if v, ok := node["variants"].([]map[string]any); ok {
				for _, vm := range v {
					rawVariants = append(rawVariants, vm)
				}
			}

			for _, rv := range rawVariants {
				vMap, ok := rv.(map[string]any)
				if !ok {
					continue
				}
				vID, _ := vMap["id"].(string)
				if vID == "" {
					continue
				}

				vImgURL := ""
				if vImg, ok := vMap["image"].(map[string]any); ok {
					vImgURL, _ = vImg["url"].(string)
				}
				if vImgURL == "" {
					vImgURL = mainImageURL
				}

				if vImgURL != "" {
					shopifyImageMap[vID] = map[string]string{
						"sourceUrl": vImgURL,
					}
				}
			}

			if len(shopifyImageMap) > 0 {
				if mapData, err := json.Marshal(shopifyImageMap); err == nil {
					optionsPayload["shopifyImage"] = string(mapData)
				}
			}

			if mainImageURL != "" {
				optionsPayload["shopifyImageSourceUrl"] = mainImageURL
			}

			localEntity := gidToResource[requestedGID]
			category := "product"
			if localEntity != nil && localEntity.CategorySlug != nil {
				category = *localEntity.CategorySlug
			}

			resource := &content.ResourceNode{
				Title:          title,
				Slug:           fmt.Sprintf("%s-%s", category, handle),
				CategorySlug:   &category,
				OneLiner:       oneLiner,
				NodeType:       "Resource",
				OptionsPayload: optionsPayload,
			}

			op, err := s.resourceService.UpsertShopifyResource(tenantCtx, resource)
			if err != nil {
				s.logger.System().Error("Failed to reconcile product", "error", err, "handle", handle)
				continue
			}

			if op != "none" {
				reconciledCount++
			}
		}
	}

	return totalProcessed, reconciledCount, deletedCount, nil
}

// CreateCart creates a new cart via the Shopify Storefront API with optional attributes and identity.
func (s *ShopifyService) CreateCart(tenantCtx *tenant.Context, lines []CartLineInput, attributes []AttributeInput, email string) (string, error) {
	token := tenantCtx.Config.ShopifyStorefrontToken
	domain := tenantCtx.Config.ShopifyStoreDomain
	apiVersion := tenantCtx.Config.ShopifyAPIVersion

	if token == "" || domain == "" {
		return "", fmt.Errorf("shopify credentials (token/domain) missing for tenant %s", tenantCtx.TenantID)
	}
	if apiVersion == "" {
		return "", fmt.Errorf("shopify api version not configured for tenant %s", tenantCtx.TenantID)
	}

	cleanDomain := strings.TrimSuffix(domain, "/")
	if !strings.HasPrefix(cleanDomain, "http") {
		cleanDomain = "https://" + cleanDomain
	}
	url := fmt.Sprintf("%s/api/%s/graphql.json", cleanDomain, apiVersion)

	query := `mutation cartCreate($input: CartInput!) {
		cartCreate(input: $input) {
			cart {
				checkoutUrl
			}
			userErrors {
				field
				message
			}
		}
	}`

	cartInput := map[string]any{
		"lines": lines,
	}

	if len(attributes) > 0 {
		cartInput["attributes"] = attributes
	}

	if email != "" {
		cartInput["buyerIdentity"] = map[string]any{
			"email": email,
		}
	}

	variables := map[string]any{
		"input": cartInput,
	}

	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal cart request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Shopify-Storefront-Private-Token", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("shopify cart request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.System().Warn("Failed to close Shopify cart response body", "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("shopify api error: %s %s", resp.Status, string(b))
	}

	var response struct {
		Data struct {
			CartCreate struct {
				Cart struct {
					CheckoutURL string `json:"checkoutUrl"`
				} `json:"cart"`
				UserErrors []struct {
					Message string `json:"message"`
				} `json:"userErrors"`
			} `json:"cartCreate"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Errors) > 0 {
		return "", fmt.Errorf("graphql errors: %v", response.Errors)
	}

	if len(response.Data.CartCreate.UserErrors) > 0 {
		return "", fmt.Errorf("shopify user error: %s", response.Data.CartCreate.UserErrors[0].Message)
	}

	return response.Data.CartCreate.Cart.CheckoutURL, nil
}
