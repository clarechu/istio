// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package status

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"istio.io/istio/pilot/cmd/pilot-agent/handle"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/pkg/env"

	"istio.io/istio/pilot/cmd/pilot-agent/status/ready"
	"istio.io/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	UpstreamLocalAddressIPv4 = &net.TCPAddr{IP: net.ParseIP("127.0.0.6")}
	UpstreamLocalAddressIPv6 = &net.TCPAddr{IP: net.ParseIP("::6")}
)

const (
	// readyPath is for the pilot agent readiness itself.
	readyPath = "/healthz/ready"
	// quitPath is to notify the pilot agent to quit.
	quitPath = "/quitquitquit"
	// KubeAppProberEnvName is the name of the command line flag for pilot agent to pass app prober config.
	// The json encoded string to pass app HTTP probe information from injector(istioctl or webhook).
	// For example, ISTIO_KUBE_APP_PROBERS='{"/app-health/httpbin/livez":{"httpGet":{"path": "/hello", "port": 8080}}.
	// indicates that httpbin container liveness prober port is 8080 and probing path is /hello.
	// This environment variable should never be set manually.
	KubeAppProberEnvName = "ISTIO_KUBE_APP_PROBERS"
)

var PrometheusScrapingConfig = env.RegisterStringVar("ISTIO_PROMETHEUS_ANNOTATIONS", "", "")

var (
	appProberPattern = regexp.MustCompile(`^/app-health/[^/]+/(livez|readyz)$`)
)

const (
	localHostIPv4 = "127.0.0.1"
	localHostIPv6 = "[::1]"
)

// KubeAppProbers holds the information about a Kubernetes pod prober.
// It's a map from the prober URL path to the Kubernetes Prober config.
// For example, "/app-health/hello-world/livez" entry contains liveness prober config for
// container "hello-world".
type KubeAppProbers map[string]*Prober

// Prober represents a single container prober
type Prober struct {
	HTTPGet        *corev1.HTTPGetAction   `json:"httpGet"`
	TimeoutSeconds int32                   `json:"timeoutSeconds,omitempty"`
	TCPSocket      *corev1.TCPSocketAction `json:"tcpSocket,omitempty"`
}

// Config for the status server.
type Config struct {
	LocalHostAddr string
	// KubeAppProbers is a json with Kubernetes application prober config encoded.
	KubeAppProbers string
	NodeType       model.NodeType
	StatusPort     uint16
	AdminPort      uint16
	IPv6           bool
	NoEnvoy        bool
	PodIP          string
	Context        context.Context
}

// Server provides an endpoint for handling status probes.
type Server struct {
	ready                 *ready.Probe
	prometheus            *PrometheusScrapeConfiguration
	mutex                 sync.RWMutex
	appKubeProbers        KubeAppProbers
	appProbeClient        map[string]*http.Client
	statusPort            uint16
	lastProbeSuccessful   bool
	envoyStatsPort        int
	upstreamLocalAddress  *net.TCPAddr
	appProbersDestination string
	localhost             string
}

// wrapIPv6 wraps the ip into "[]" in case of ipv6
func wrapIPv6(ipAddr string) string {
	addr := net.ParseIP(ipAddr)
	if addr == nil {
		return ipAddr
	}
	if addr.To4() != nil {
		return ipAddr
	}
	return fmt.Sprintf("[%s]", ipAddr)
}

// NewServer creates a new status server.
func NewServer(config Config) (*Server, error) {
	localhost := localHostIPv4
	upstreamLocalAddress := UpstreamLocalAddressIPv4
	if config.IPv6 {
		localhost = localHostIPv6
		upstreamLocalAddress = UpstreamLocalAddressIPv6
	}
	probes := make([]*ready.Probe, 0)
	if !config.NoEnvoy {
		probes = append(probes, &ready.Probe{
			LocalHostAddr: localhost,
			AdminPort:     config.AdminPort,
			Context:       config.Context,
			NoEnvoy:       config.NoEnvoy,
		})
	}
	s := &Server{
		statusPort:            config.StatusPort,
		upstreamLocalAddress:  upstreamLocalAddress,
		appProbersDestination: wrapIPv6(config.PodIP),
		ready: &ready.Probe{
			LocalHostAddr: config.LocalHostAddr,
			AdminPort:     config.AdminPort,
			NodeType:      config.NodeType,
		},
		envoyStatsPort: 15090,
	}
	if config.KubeAppProbers == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(config.KubeAppProbers), &s.appKubeProbers); err != nil {
		return nil, fmt.Errorf("failed to decode app prober err = %v, json string = %v", err, config.KubeAppProbers)
	}

	s.appProbeClient = make(map[string]*http.Client, len(s.appKubeProbers))
	// Validate the map key matching the regex pattern.
	/*	for path, prober := range s.appKubeProbers {
		err := validateAppKubeProber(path, prober)
		if err != nil {
			return nil, err
		}
		if prober.HTTPGet != nil {
			d := &net.Dialer{
				LocalAddr: s.upstreamLocalAddress,
			}
			// Construct a http client and cache it in order to reuse the connection.
			s.appProbeClient[path] = &http.Client{
				Timeout: time.Duration(prober.TimeoutSeconds) * time.Second,
				// We skip the verification since kubelet skips the verification for HTTPS prober as well
				// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#configure-probes
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					DialContext:     d.DialContext,
				},
			}
		}
	}*/

	// Enable prometheus server if its configured and a sidecar
	// Because port 15020 is exposed in the gateway Services, we cannot safely serve this endpoint
	// If we need to do this in the future, we should use envoy to do routing or have another port to make this internal
	// only. For now, its not needed for gateway, as we can just get Envoy stats directly, but if we
	// want to expose istio-agent metrics we may want to revisit this.
	if cfg, f := PrometheusScrapingConfig.Lookup(); config.NodeType == model.SidecarProxy && f {
		var prom PrometheusScrapeConfiguration
		if err := json.Unmarshal([]byte(cfg), &prom); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %v", PrometheusScrapingConfig.Name, err)
		}
		log.Infof("Prometheus scraping configuration: %v", prom)
		s.prometheus = &prom
		if s.prometheus.Path == "" {
			s.prometheus.Path = "/metrics"
		}
		if s.prometheus.Port == "" {
			s.prometheus.Port = "80"
		}
		if s.prometheus.Port == strconv.Itoa(int(config.StatusPort)) {
			return nil, fmt.Errorf("invalid prometheus scrape configuration: "+
				"application port is the same as agent port, which may lead to a recursive loop. "+
				"Ensure pod does not have prometheus.io/port=%d label, or that injection is not happening multiple times", config.StatusPort)
		}
	}

	if config.KubeAppProbers == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(config.KubeAppProbers), &s.appKubeProbers); err != nil {
		return nil, fmt.Errorf("failed to decode app prober err = %v, json string = %v", err, config.KubeAppProbers)
	}

	s.appProbeClient = make(map[string]*http.Client, len(s.appKubeProbers))
	// Validate the map key matching the regex pattern.
	for path, prober := range s.appKubeProbers {
		err := validateAppKubeProber(path, prober)
		if err != nil {
			return nil, err
		}
		if prober.HTTPGet != nil {
			d := &net.Dialer{
				LocalAddr: s.upstreamLocalAddress,
			}
			// Construct a http client and cache it in order to reuse the connection.
			s.appProbeClient[path] = &http.Client{
				Timeout: time.Duration(prober.TimeoutSeconds) * time.Second,
				// We skip the verification since kubelet skips the verification for HTTPS prober as well
				// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#configure-probes
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					DialContext:     d.DialContext,
				},
			}
		}
	}
	return s, nil
}

// FormatProberURL returns a pair of HTTP URLs that pilot agent will serve to take over Kubernetes
// app probers.
func FormatProberURL(container string) (string, string) {
	return fmt.Sprintf("/app-health/%v/readyz", container),
		fmt.Sprintf("/app-health/%v/livez", container)
}

// Run opens a the status port and begins accepting probes.
// this http server
func (s *Server) Run(ctx context.Context) {
	log.Infof("Opening status port %d\n", s.statusPort)

	mux := http.NewServeMux()

	// Add the handler for ready probes.
	mux.HandleFunc(readyPath, s.handleReadyProbe)
	mux.HandleFunc(`/stats/prometheus`, s.handleStats)
	mux.HandleFunc(quitPath, s.handleQuit)
	mux.HandleFunc("/app-health/", s.handleAppProbe)
	mux.HandleFunc("/istio-clean-iptables", handle.IstioCleanIptables)
	mux.HandleFunc("/istio-iptables", handle.IstioIptables)
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", s.statusPort))
	if err != nil {
		log.Errorf("Error listening on status port: %v", err.Error())
		return
	}
	// for testing.
	if s.statusPort == 0 {
		addrs := strings.Split(l.Addr().String(), ":")
		allocatedPort, _ := strconv.Atoi(addrs[len(addrs)-1])
		s.mutex.Lock()
		s.statusPort = uint16(allocatedPort)
		s.mutex.Unlock()
	}
	defer l.Close()

	go func() {
		if err := http.Serve(l, mux); err != nil {
			log.Errora(err)
			// If the server errors then pilot-agent can never pass readiness or liveness probes
			// Therefore, trigger graceful termination by sending SIGTERM to the binary pid
			notifyExit()
		}
	}()

	// Wait for the agent to be shut down.
	<-ctx.Done()
	log.Info("Status server has successfully terminated")
}

func validateAppKubeProber(path string, prober *Prober) error {
	if !appProberPattern.Match([]byte(path)) {
		return fmt.Errorf(`invalid path, must be in form of regex pattern %v`, appProberPattern)
	}
	if prober.HTTPGet == nil && prober.TCPSocket == nil {
		return fmt.Errorf(`invalid prober type, must be of type httpGet or tcpSocket`)
	}
	if prober.HTTPGet != nil && prober.TCPSocket != nil {
		return fmt.Errorf(`invalid prober, type must be either httpGet or tcpSocket`)
	}
	if prober.HTTPGet != nil && prober.HTTPGet.Port.Type != intstr.Int {
		return fmt.Errorf("invalid prober config for %v, the port must be int type", path)
	}
	if prober.TCPSocket != nil && prober.TCPSocket.Port.Type != intstr.Int {
		return fmt.Errorf("invalid prober config for %v, the port must be int type", path)
	}
	return nil
}

func (s *Server) handleReadyProbe(w http.ResponseWriter, _ *http.Request) {
	err := s.ready.Check()

	s.mutex.Lock()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)

		log.Warnf("Envoy proxy is NOT ready: %s", err.Error())
		s.lastProbeSuccessful = false
	} else {
		w.WriteHeader(http.StatusOK)

		if !s.lastProbeSuccessful {
			log.Info("Envoy proxy is ready")
		}
		s.lastProbeSuccessful = true
	}
	s.mutex.Unlock()
}

func isRequestFromLocalhost(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}

	userIP := net.ParseIP(ip)
	return userIP.IsLoopback()
}

type PrometheusScrapeConfiguration struct {
	Scrape string `json:"scrape"`
	Path   string `json:"path"`
	Port   string `json:"port"`
}

// handleStats handles prometheus stats scraping. This will scrape envoy metrics, and, if configured,
// the application metrics and merge them together.
// The merge here is a simple string concatenation. This works for almost all cases, assuming the application
// is not exposing the same metrics as Envoy.
// TODO(https://github.com/istio/istio/issues/22825) expose istio-agent stats here as well
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var envoy, application []byte
	var err error
	if envoy, err = s.scrape(fmt.Sprintf("http://localhost:%d/stats/prometheus", s.envoyStatsPort), r.Header); err != nil {
		log.Errora(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if s.prometheus != nil {
		url := fmt.Sprintf("http://localhost:%s%s", s.prometheus.Port, s.prometheus.Path)
		if application, err = s.scrape(url, r.Header); err != nil {
			log.Errora(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	if _, err := w.Write(envoy); err != nil {
		log.Errorf("failed to write envoy metrics: %v", err)
		return
	}
	if _, err := w.Write(application); err != nil {
		log.Errorf("failed to write application metrics: %v", err)
	}
}

func applyHeaders(into http.Header, from http.Header, keys ...string) {
	for _, key := range keys {
		val := from.Get(key)
		if val != "" {
			into.Set(key, val)
		}
	}
}

// getHeaderTimeout parse a string like (1.234) representing number of seconds
func getHeaderTimeout(timeout string) (time.Duration, error) {
	timeoutSeconds, err := strconv.ParseFloat(timeout, 64)
	if err != nil {
		return 0 * time.Second, err
	}

	return time.Duration(timeoutSeconds * 1e9), nil
}

// scrape will send a request to the provided url to scrape metrics from
// This will attempt to mimic some of Prometheus functionality by passing some of the headers through
// such as timeout and user agent
func (s *Server) scrape(url string, header http.Header) ([]byte, error) {
	ctx := context.Background()
	if timeoutString := header.Get("X-Prometheus-Scrape-Timeout-Seconds"); timeoutString != "" {
		timeout, err := getHeaderTimeout(timeoutString)
		if err != nil {
			log.Warnf("Failed to parse timeout header %v: %v", timeoutString, err)
		} else {
			c, cancel := context.WithTimeout(ctx, timeout)
			ctx = c
			defer cancel()
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req.Header, header, "Accept",
		"User-Agent",
		"X-Prometheus-Scrape-Timeout-Seconds",
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error scraping %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error scraping %s, status code: %v", url, resp.StatusCode)
	}
	defer resp.Body.Close()
	metrics, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", url, err)
	}

	return metrics, nil
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if !isRequestFromLocalhost(r) {
		http.Error(w, "Only requests from localhost are allowed", http.StatusForbidden)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
	log.Infof("handling %s, notifying pilot-agent to exit", quitPath)
	notifyExit()
}

func (s *Server) handleAppProbe(w http.ResponseWriter, req *http.Request) {
	// Validate the request first.
	path := req.URL.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + req.URL.Path
	}
	prober, exists := s.appKubeProbers[path]
	if !exists {
		log.Errorf("Prober does not exists url %v", path)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf("app prober config does not exists for %v", path)))
		return
	}

	if prober.HTTPGet != nil {
		s.handleAppProbeHTTPGet(w, req, prober, path)
	}
	if prober.TCPSocket != nil {
		s.handleAppProbeTCPSocket(w, prober)
	}
}

func (s *Server) handleAppProbeHTTPGet(w http.ResponseWriter, req *http.Request, prober *Prober, path string) {
	proberPath := prober.HTTPGet.Path
	if !strings.HasPrefix(proberPath, "/") {
		proberPath = "/" + proberPath
	}
	var url string
	if prober.HTTPGet.Scheme == corev1.URISchemeHTTPS {
		url = fmt.Sprintf("https://%s:%v%s", s.appProbersDestination, prober.HTTPGet.Port.IntValue(), proberPath)
	} else {
		url = fmt.Sprintf("http://%s:%v%s", s.appProbersDestination, prober.HTTPGet.Port.IntValue(), proberPath)
	}
	appReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorf("Failed to create request to probe app %v, original url %v", err, path)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Forward incoming headers to the application.
	for name, values := range req.Header {
		newValues := make([]string, len(values))
		copy(newValues, values)
		appReq.Header[name] = newValues
	}

	// If there are custom HTTPHeaders, it will override the forwarding header
	if headers := prober.HTTPGet.HTTPHeaders; len(headers) != 0 {
		for _, h := range headers {
			delete(appReq.Header, h.Name)
		}
		for _, h := range headers {
			if h.Name == "Host" || h.Name == ":authority" {
				// Probe has specific host header override; honor it
				appReq.Host = h.Value
				appReq.Header.Set(h.Name, h.Value)
			} else {
				appReq.Header.Add(h.Name, h.Value)
			}
		}
	}

	// get the http client must exist because
	httpClient := s.appProbeClient[path]

	// Send the request.
	response, err := httpClient.Do(appReq)
	if err != nil {
		log.Errorf("Request to probe app failed: %v, original URL path = %v\napp URL path = %v", err, path, proberPath)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() {
		// Drain and close the body to let the Transport reuse the connection
		_, _ = io.Copy(ioutil.Discard, response.Body)
		_ = response.Body.Close()
	}()

	// We only write the status code to the response.
	w.WriteHeader(response.StatusCode)
}

func (s *Server) handleAppProbeTCPSocket(w http.ResponseWriter, prober *Prober) {
	port := prober.TCPSocket.Port.IntValue()
	timeout := time.Duration(prober.TimeoutSeconds) * time.Second

	d := &net.Dialer{
		LocalAddr: s.upstreamLocalAddress,
		Timeout:   timeout,
	}

	conn, err := d.Dial("tcp", fmt.Sprintf("%s:%d", s.appProbersDestination, port))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
		conn.Close()
	}
}

// notifyExit sends SIGTERM to itself
func notifyExit() {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		log.Errora(err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		log.Errorf("failed to send SIGTERM to self: %v", err)
	}
}
