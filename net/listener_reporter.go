// Package net is based on [go-conntrack](https://github.com/mwitkow/go-conntrack)
package net

import "github.com/prometheus/client_golang/prometheus"

const (
	listenerLabel = "listener_name"
)

var (
	defaultPromLabels = []string{
		// What's the listener name
		listenerLabel,
		// Which backend the listener belong to
		"backend",
		// What's the wormhole cluster
		"cluster",
		// What's the wormhole node
		"node",
	}

	listenerAcceptedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "net",
			Name:      "listener_conn_accepted_total",
			Help:      "Total number of connections opened to the listener of a given name.",
		}, defaultPromLabels)

	listenerClosedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "net",
			Name:      "listener_conn_closed_total",
			Help:      "Total number of connections closed that were made to the listener of a given name.",
		}, defaultPromLabels)

	connDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  "wormhole",
			Subsystem:  "net",
			Name:       "conn_duration_seconds",
			Help:       "Duration in seconds of connections.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, []string{listenerLabel, "cluster"})

	connRcvdBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "net",
			Name:      "conn_rcvd_bytes",
			Help:      "Number of bytes received from connections.",
		}, defaultPromLabels)

	connSentBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "net",
			Name:      "conn_sent_bytes",
			Help:      "Number of bytes sent to ingress connections.",
		}, defaultPromLabels)
)

func init() {
	prometheus.MustRegister(listenerAcceptedTotal)
	prometheus.MustRegister(listenerClosedTotal)
	prometheus.MustRegister(connDuration)
	prometheus.MustRegister(connRcvdBytes)
	prometheus.MustRegister(connSentBytes)
}

// preRegisterListener pre-populates Prometheus labels for the given listener name, to avoid Prometheus missing labels issue.
func preRegisterListenerMetrics(listenerName string, labels map[string]string) error {
	var err error
	if _, err = listenerAcceptedTotal.GetMetricWith(getLabels(listenerName, labels)); err != nil {
		return err
	}
	if _, err = listenerClosedTotal.GetMetricWith(getLabels(listenerName, labels)); err != nil {
		return err
	}
	if _, err = connDuration.GetMetricWith(getSummaryLabels(listenerName, labels)); err != nil {
		return err
	}
	if _, err = connRcvdBytes.GetMetricWith(getLabels(listenerName, labels)); err != nil {
		return err
	}
	if _, err = connSentBytes.GetMetricWith(getLabels(listenerName, labels)); err != nil {
		return err
	}
	return nil
}

func reportListenerConnAccepted(listenerName string, labels map[string]string) {
	listenerAcceptedTotal.With(getLabels(listenerName, labels)).Inc()
}

func reportListenerConnClosed(listenerName string, labels map[string]string) {
	listenerClosedTotal.With(getLabels(listenerName, labels)).Inc()
}

func reportConnDuration(listenerName string, labels map[string]string, duration float64) {
	connDuration.With(getSummaryLabels(listenerName, labels)).Observe(duration)
}

func reportConnRcvdBytes(listenerName string, labels map[string]string, bytes float64) {
	connRcvdBytes.With(getLabels(listenerName, labels)).Add(bytes)
}

func reportConnSentBytes(listenerName string, labels map[string]string, bytes float64) {
	connSentBytes.With(getLabels(listenerName, labels)).Add(bytes)
}

func getLabels(name string, labels map[string]string) prometheus.Labels {
	promLabels := prometheus.Labels{}
	promLabels[listenerLabel] = name
	for k, v := range labels {
		if isPromLabel(k) {
			promLabels[k] = v
		}
	}
	return promLabels
}

func getSummaryLabels(name string, labels map[string]string) prometheus.Labels {
	promLabels := prometheus.Labels{}
	promLabels[listenerLabel] = name
	promLabels["cluster"] = labels["cluster"]
	return promLabels
}

func isPromLabel(l string) bool {
	for _, label := range defaultPromLabels {
		if label == l {
			return true
		}
	}
	return false
}
