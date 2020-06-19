/*
Copyright 2019, Oath Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package clusterconfig

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

// TrustedSource is a source that can be trusted using CA certs.
type TrustedSource string

// SANIPAddrType is the SAN IP to be included on the X.509 CSR
type SANIPAddrType string

// ClusterConfiguration is the config for the cluster
type ClusterConfiguration struct {
	ClusterDNSSuffix            string                   `json:"cluster-dns-suffix"`       // the cluster specific DNS suffix for control plane services
	ControlDNSSuffix            string                   `json:"control-dns-suffix"`       // the DNS suffix for self-hosted control plane services
	DNSSuffix                   string                   `json:"dns-suffix"`               // the DNS suffix for kube-dns as well as Athenz minted certs
	IdentityService             string                   `json:"identity-service"`         // namespace/name of the identityd service
	AdminDomain                 string                   `json:"admin-domain"`             // the admin domain used for namespace to domain mapping
	ZMSEndpoint                 string                   `json:"zms-endpoint"`             // ZMS endpoint with /v1 path
	ZTSEndpoint                 string                   `json:"zts-endpoint"`             // ZTS endpoint with /v1 path
	ZTSCommonName               string                   `json:"zts-commonname"`           // ZTS common name to verify creds during the TLS handshake
	CSRSubject                  pkix.Name                `json:"csr-subject"`              // Subject fields on the CSR to use
	CSRSubjectAltNameIPAddrType SANIPAddrType            `json:"csr-san-ip"`               // SAN IP to include on the CSR (either PodIP(default) or HostIP)
	ProviderService             string                   `json:"provider-service"`         // the provider service as a fully qualified Athenz name
	AuthHeader                  string                   `json:"auth-header"`              // auth header name for Athenz requests
	SystemNamespaces            []string                 `json:"system-namespaces"`        // system namespaces in addition to those starting with "kube-"
	TrustRoots                  map[TrustedSource]string `json:"trust-roots"`              // key-value pairs containing file paths to CA trust bundles for TLS entities
	ClusterLevelDenyUsers       []string                 `json:"cluster-level-deny-users"` // list of users not allowed to access control plane components
}

// CmdLine provides a mechanism to return the cluster configuration for a
// CLI app.
func CmdLine(f *flag.FlagSet) func() (*ClusterConfiguration, error) {
	file := os.Getenv("CLUSTER_CONFIG_FILE")
	f.StringVar(&file, "config", file, "cluster config file path (blank to use in-cluster config)")
	return func() (*ClusterConfiguration, error) {
		if file == "" {
			log.Println("no config file specified, load in-cluster config")
			return loadInClusterConfig()
		}
		b, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var c ClusterConfiguration
		if err := yaml.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		return &c, nil
	}
}

const (
	cmNamespace = "kube-yahoo"
	cmName      = "cluster-config"
	keyName     = "config.yaml"
	caCertFile  = "ca.crt"
	tokenFile   = "token"
	timeout     = 10 * time.Second
)

var (
	saDir      = "/var/run/secrets/kubernetes.io/serviceaccount"
	apiBaseURL = "https://kubernetes.default"
)

type response struct {
	Data map[string]string `json:"data"`
}

// getURL returns the URL to get the config map
func getURL() string {
	return fmt.Sprintf("%s/api/v1/namespaces/%s/configmaps/%s", apiBaseURL, cmNamespace, cmName)
}

// getTLSConfig returns the TLS configuration for the API call
func getTLSConfig() (*tls.Config, error) {
	file := filepath.Join(saDir, caCertFile)
	caPEM, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	b := pool.AppendCertsFromPEM(caPEM)
	if !b {
		return nil, fmt.Errorf("no valid CA certs in %s", file)
	}
	return &tls.Config{
		RootCAs: pool,
	}, nil
}

// getToken returns the service account token.
func getToken() (string, error) {
	file := filepath.Join(saDir, tokenFile)
	tok, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	t := string(tok)
	return strings.TrimRight(t, "\r\n"), nil
}

// loadInClusterConfig loads the config map in the context of
// a workload running on the cluster.
func loadInClusterConfig() (*ClusterConfiguration, error) {
	url := getURL()
	tlsConfig, err := getTLSConfig()
	if err != nil {
		return nil, errors.Wrap(err, "get TLS config")
	}
	tok, err := getToken()
	if err != nil {
		return nil, errors.Wrap(err, "get token")
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}
	var r response
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, errors.Wrap(err, "json unmarshal")
	}
	yml, ok := r.Data[keyName]
	if !ok {
		return nil, fmt.Errorf("unable to find key %s in config map", keyName)
	}
	var c ClusterConfiguration
	if err := yaml.Unmarshal([]byte(yml), &c); err != nil {
		return nil, errors.Wrap(err, "yaml unmarshal")
	}
	return &c, nil
}
