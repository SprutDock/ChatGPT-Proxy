package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type APIClient struct {
	baseURL string
}

type EnrollmentResult struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	DeviceID         string `json:"device_id"`
	CertificateChain string `json:"certificate_chain"`
}

type RenewalResult struct {
	RequestID        string `json:"request_id"`
	Serial           string `json:"serial"`
	CertificateChain string `json:"certificate_chain"`
}

func newAPIClient() *APIClient {
	return &APIClient{baseURL: apiBaseURL}
}

func (client *APIClient) post(path string, body any, certificate *tls.Certificate, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, client.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{
		Proxy:           nil,
		DialContext:     (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if certificate != nil {
		transport.TLSClientConfig.Certificates = []tls.Certificate{*certificate}
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("enrollment service is unavailable: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 512*1024))
	if err != nil {
		return fmt.Errorf("enrollment service response could not be read: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var value struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(responseBody, &value) == nil && value.Error != "" {
			return fmt.Errorf("%s", value.Error)
		}
		return fmt.Errorf("enrollment service returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("enrollment service response is invalid: %w", err)
	}
	return nil
}

func (client *APIClient) enroll(invitation, requestID, credential, csr string) (EnrollmentResult, error) {
	var result EnrollmentResult
	err := client.post("/v1/enroll", map[string]string{
		"invitation":            invitation,
		"request_id":            requestID,
		"enrollment_credential": credential,
		"csr":                   csr,
	}, nil, &result)
	return result, err
}

func (client *APIClient) renew(requestID, csr string, current *tls.Certificate) (RenewalResult, error) {
	var result RenewalResult
	err := client.post("/v1/renew", map[string]string{
		"request_id": requestID,
		"csr":        csr,
	}, current, &result)
	return result, err
}

func (client *APIClient) confirmRotation(requestID string, current *tls.Certificate) error {
	var result map[string]any
	return client.post("/v1/rotation-confirm", map[string]string{"request_id": requestID}, current, &result)
}
