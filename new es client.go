package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"orenctl/internal/app/helper"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/spf13/viper"
)

type IESClient interface {
	Search(aliasName string, query helper.Map, size int) (helper.Map, error)
	BulkIndexDocuments(alias string, docs []interface{}) error
	BulkIndexDocumentsWithRetry(alias string, docs []interface{}, retries int, retryInterval time.Duration) error
}

type ESClient struct {
	Client *elasticsearch.Client
}

func NewClient(addresses []string) (*ESClient, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &ESClient{Client: es}, nil
}

func (es *ESClient) Search(aliasName string, query helper.Map, size int) (helper.Map, error) {
	// Convert the query map to JSON
	queryBody, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Use alias name for search
	searchRequest := esapi.SearchRequest{
		Index: []string{viper.GetString("elastic.event.prefix") + aliasName},
		Body:  strings.NewReader(string(queryBody)),
		Size:  &size,
	}

	res, err := searchRequest.Do(context.Background(), es.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search request: %w", err)
	}
	defer res.Body.Close()

	var result helper.Map
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return result, nil
}

// BulkIndexDocuments indexes multiple documents using the alias
func (c *ESClient) BulkIndexDocuments(alias string, docs []interface{}) error {
	// First, get the write index for the alias
	writeIndex, err := c.getWriteIndexForAlias(alias)
	if err != nil {
		return fmt.Errorf("failed to get write index for alias: %w", err)
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": writeIndex, // Use the actual write index
			},
		}
		if err := json.NewEncoder(&buf).Encode(action); err != nil {
			return err
		}

		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			return err
		}
	}

	res, err := c.Client.Bulk(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error bulk indexing documents: %s", res.String())
	}

	var bulkResponse map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&bulkResponse); err != nil {
		return err
	}

	if bulkResponse["errors"].(bool) {
		return fmt.Errorf("bulk indexing encountered errors: %v", bulkResponse)
	}

	return nil
}

// getWriteIndexForAlias gets the current write index for an alias
func (c *ESClient) getWriteIndexForAlias(alias string) (string, error) {
	aliasName := viper.GetString("elastic.event.prefix") + alias
	res, err := c.Client.Indices.GetAlias(
		c.Client.Indices.GetAlias.WithName(aliasName),
	)
	if err != nil {
		return "", fmt.Errorf("failed to get alias info: %w", err)
	}
	defer res.Body.Close()

	var aliasResponse map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&aliasResponse); err != nil {
		return "", fmt.Errorf("failed to decode alias response: %w", err)
	}

	// Find the write index
	for indexName, indexData := range aliasResponse {
		if indexInfo, ok := indexData.(map[string]interface{})["aliases"].(map[string]interface{})[aliasName]; ok {
			if writeIndex, ok := indexInfo.(map[string]interface{})["is_write_index"]; ok && writeIndex.(bool) {
				return indexName, nil
			}
		}
	}

	return "", fmt.Errorf("no write index found for alias %s", aliasName)
}

func (c *ESClient) BulkIndexDocumentsWithRetry(alias string, docs []interface{}, retries int, retryInterval time.Duration) error {
	var err error

	for attempt := 0; attempt < retries; attempt++ {
		if err = c.bulkIndex(alias, docs); err == nil {
			return nil
		}

		fmt.Printf("Bulk indexing failed (attempt %d/%d). Retrying in %v...\n", attempt+1, retries, retryInterval)
		time.Sleep(retryInterval)
	}

	return fmt.Errorf("bulk indexing failed after %d retries: %v", retries, err)
}

func (c *ESClient) bulkIndex(alias string, docs []interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic: %v", r)
			err = fmt.Errorf("panic occurred: %v", r)
		}
	}()

	// Get the write index for the alias
	writeIndex, err := c.getWriteIndexForAlias(alias)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		if err = c.encodeActionAndDocument(&buf, writeIndex, doc); err != nil {
			return err
		}
	}

	res, err := c.Client.Bulk(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return c.handleBulkResponse(res)
}

func (c *ESClient) encodeActionAndDocument(buf *bytes.Buffer, writeIndex string, doc interface{}) error {
	// Action - use the actual write index instead of the alias
	action := map[string]interface{}{
		"index": map[string]interface{}{
			"_index": writeIndex,
		},
	}
	if err := json.NewEncoder(buf).Encode(action); err != nil {
		return err
	}
	return json.NewEncoder(buf).Encode(doc)
}

func (c *ESClient) handleBulkResponse(res *esapi.Response) error {
	if res.IsError() {
		return fmt.Errorf("bulk indexing failed: %s", res.Status())
	}

	var bulkResponse map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&bulkResponse); err != nil {
		return err
	}

	if bulkResponse["errors"].(bool) {
		return fmt.Errorf("bulk indexing failed: errors in response")
	}

	return nil
}