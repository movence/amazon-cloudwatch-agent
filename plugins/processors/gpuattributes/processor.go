// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package gpuattributes

import (
	"context"
	"encoding/json"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/aws/amazon-cloudwatch-agent/internal/containerinsightscommon"
)

const (
	gpuMetricIdentifier      = "_gpu_"
	gpuContainerMetricPrefix = "container_"
	gpuPodMetricPrefix       = "pod_"
	gpuNodeMetricPrefix      = "node_"
)

// schemas at each resource level
// - Container Schema
//   - ClusterName
//   - ClusterName, Namespace, PodName, ContainerName
//   - ClusterName, Namespace, PodName, FullPodName, ContainerName
//   - ClusterName, Namespace, PodName, FullPodName, ContainerName, GpuDevice
//
// - Pod
//   - ClusterName
//   - ClusterName, Namespace
//   - ClusterName, Namespace, Service
//   - ClusterName, Namespace, PodName
//   - ClusterName, Namespace, PodName, FullPodName
//   - ClusterName, Namespace, PodName, FullPodName, GpuDevice
//
// - Node
//   - ClusterName
//   - ClusterName, InstanceIdKey, NodeName
//   - ClusterName, InstanceIdKey, NodeName, GpuDevice
var nodeLabels = []string{
	containerinsightscommon.ClusterNameKey,
	containerinsightscommon.InstanceIdKey,
	containerinsightscommon.GpuDeviceKey,
	containerinsightscommon.MetricType,
	containerinsightscommon.NodeNameKey,
	containerinsightscommon.VersionKey,
	containerinsightscommon.SourcesKey,
	containerinsightscommon.Timestamp,
}
var podLabels = append([]string{
	containerinsightscommon.K8sNamespace,
	containerinsightscommon.FullPodNameKey,
	containerinsightscommon.PodNameKey,
	containerinsightscommon.TypeService,
	containerinsightscommon.GpuUniqueId,
}, nodeLabels...)
var containerLabels = append([]string{
	containerinsightscommon.ContainerNamekey,
}, podLabels...)

var nodeK8sLabels = []string{containerinsightscommon.HostKey}
var podK8sLabels = append([]string{
	"host",
	"labels",
	"pod_id",
	"pod_name",
	"pod_owners",
	"namespace",
}, nodeK8sLabels...)
var containerK8sLabels = append([]string{
	"container_name",
	"containerd",
}, podK8sLabels...)

type gpuAttributesProcessor struct {
	*Config
	logger *zap.Logger
}

func newGpuAttributesProcessor(config *Config, logger *zap.Logger) *gpuAttributesProcessor {
	d := &gpuAttributesProcessor{
		Config: config,
		logger: logger,
	}
	return d
}

func (d *gpuAttributesProcessor) processMetrics(_ context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rs := rms.At(i)
		ilms := rs.ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			ils := ilms.At(j)
			metrics := ils.Metrics()
			for k := 0; k < metrics.Len(); k++ {
				m := metrics.At(k)
				d.processMetricAttributes(m)
			}
		}
	}
	return md, nil
}

func (d *gpuAttributesProcessor) processMetricAttributes(m pmetric.Metric) {
	// only decorate GPU metrics
	if !strings.Contains(m.Name(), gpuMetricIdentifier) {
		return
	}

	var labels, k8sBlobLabels []string
	if strings.HasPrefix(m.Name(), gpuContainerMetricPrefix) {
		labels = containerLabels
		k8sBlobLabels = containerK8sLabels
	} else if strings.HasPrefix(m.Name(), gpuPodMetricPrefix) {
		labels = podLabels
		k8sBlobLabels = podK8sLabels
	} else if strings.HasPrefix(m.Name(), gpuNodeMetricPrefix) {
		labels = nodeLabels
		k8sBlobLabels = nodeK8sLabels
	}

	labelFilter := map[string]map[string]interface{}{}
	for _, attr := range labels {
		labelFilter[attr] = nil
	}
	k8sBlobMap := map[string]interface{}{}
	for _, attr := range k8sBlobLabels {
		k8sBlobMap[attr] = nil
	}
	if len(k8sBlobMap) > 0 {
		labelFilter[containerinsightscommon.K8sKey] = k8sBlobMap
	}

	var dps pmetric.NumberDataPointSlice
	switch m.Type() {
	case pmetric.MetricTypeGauge:
		dps = m.Gauge().DataPoints()
	case pmetric.MetricTypeSum:
		dps = m.Sum().DataPoints()
	default:
		d.logger.Debug("Ignore unknown metric type", zap.String(containerinsightscommon.MetricType, m.Type().String()))
	}

	for i := 0; i < dps.Len(); i++ {
		d.filterAttributes(dps.At(i).Attributes(), labelFilter)
	}
}

func (d *gpuAttributesProcessor) filterAttributes(attributes pcommon.Map, labels map[string]map[string]interface{}) {
	if len(labels) == 0 {
		return
	}
	// remove labels that are not in the keep list
	attributes.RemoveIf(func(k string, _ pcommon.Value) bool {
		if _, ok := labels[k]; ok {
			return false
		}
		return true
	})

	// if a label has child level filter list, that means the label is map type
	// only handles map type since there are currently only map and value types with GPU
	for lk, ls := range labels {
		if len(ls) == 0 {
			continue
		}
		if av, ok := attributes.Get(lk); ok {
			// decode json formatted string value into a map then encode again after filtering elements
			var blob map[string]json.RawMessage
			strVal := av.Str()
			err := json.Unmarshal([]byte(strVal), &blob)
			if err != nil {
				d.logger.Warn("gpuAttributesProcessor: failed to unmarshal label", zap.String("label", lk))
				continue
			}
			newBlob := make(map[string]json.RawMessage)
			for bkey, bval := range blob {
				if _, ok := ls[bkey]; ok {
					newBlob[bkey] = bval
				}
			}
			bytes, err := json.Marshal(newBlob)
			if err != nil {
				d.logger.Warn("gpuAttributesProcessor: failed to marshall label", zap.String("label", lk))
				continue
			}
			attributes.PutStr(lk, string(bytes))
		}
	}
}
