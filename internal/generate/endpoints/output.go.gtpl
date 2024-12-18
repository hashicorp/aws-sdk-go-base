// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Code generated by internal/generate/endpoints/main.go; DO NOT EDIT.

package endpoints

import (
    "regexp"
)

// All known partition IDs.
const (
{{- range .Partitions }}
    // {{ .Name }}
    {{ .ID | IDToTitle}}PartitionID = "{{ .ID }}"
{{- end }}
)

// All known Region IDs.
const (
{{- range .Partitions }}
    // {{ .Name }} partition's Regions.
    {{- range .Regions }}
    // {{ .Description }}
    {{ .ID | IDToTitle}}RegionID = "{{ .ID }}"
    {{- end }}
{{- end }}
)

var (
	partitions = map[string]Partition{
{{- range .Partitions }}
        {{ .ID | IDToTitle}}PartitionID: {
            id: {{ .ID | IDToTitle}}PartitionID,
            name: "{{ .Name }}",
            dnsSuffix: "{{ .DNSSuffix }}",
            regionRegex: regexp.MustCompile(`{{ .RegionRegex }}`),
            regions: map[string]Region{
            {{- range .Regions }}
                {{ .ID | IDToTitle}}RegionID: {
                    id: {{ .ID | IDToTitle}}RegionID,
                    description: "{{ .Description }}",
                },
            {{- end }}
            },
            services: map[string]Service{
            {{- range .Services }}
                "{{ .ID }}": {
                    id: "{{ .ID }}",
                },
            {{- end }}
            },
        },
{{- end }}
    }
)