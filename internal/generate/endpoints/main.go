// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build generate
// +build generate

package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/generate/common"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/slices"
)

type PartitionDatum struct {
	ID        string
	Name      string
	DNSSuffix string
	Regions   []RegionDatum
}

type RegionDatum struct {
	ID          string
	Description string
}

type TemplateData struct {
	Partitions []PartitionDatum
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "\tmain.go <aws-sdk-go-v2-endpoints-json-url>\n\n")
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		flag.Usage()
		os.Exit(2)
	}

	inputURL := args[0]
	outputFilename := `endpoints_gen.go`
	target := map[string]any{}

	g := common.NewGenerator()

	if err := readHTTPJSON(inputURL, &target); err != nil {
		g.Fatalf("error reading JSON from %s: %s", inputURL, err)
	}

	td := TemplateData{}
	templateFuncMap := template.FuncMap{
		// KebabToTitle splits a kebab case string and returns a string with each part title cased.
		"KebabToTitle": func(s string) (string, error) {
			parts := strings.Split(s, "-")
			return strings.Join(slices.ApplyToAll(parts, func(s string) string {
				return common.FirstUpper(s)
			}), ""), nil
		},
	}

	if version, ok := target["version"].(float64); ok {
		if version != 3.0 {
			g.Fatalf("unsupported endpoints document version: %d", int(version))
		}
	} else {
		g.Fatalf("can't parse endpoints document version")
	}

	/*
		See https://github.com/aws/aws-sdk-go/blob/main/aws/endpoints/v3model.go.
		e.g.
		{
		  "partitions": [{
		    "partition": "aws",
		    "partitionName": "AWS Standard",
		    "regions" : {
		      "af-south-1" : {
		        "description" : "Africa (Cape Town)"
		      },
			  ...
		    }
			...
		   }, ...]
		}
	*/
	if partitions, ok := target["partitions"].([]any); ok {
		for _, partition := range partitions {
			if partition, ok := partition.(map[string]any); ok {
				partitionDatum := PartitionDatum{}

				if id, ok := partition["partition"].(string); ok {
					partitionDatum.ID = id
				}
				if name, ok := partition["partitionName"].(string); ok {
					partitionDatum.Name = name
				}
				if regions, ok := partition["regions"].(map[string]any); ok {
					for id, region := range regions {
						regionDatum := RegionDatum{
							ID: id,
						}

						if region, ok := region.(map[string]any); ok {
							if description, ok := region["description"].(string); ok {
								regionDatum.Description = description
							}
						}

						partitionDatum.Regions = append(partitionDatum.Regions, regionDatum)
					}
				}

				td.Partitions = append(td.Partitions, partitionDatum)
			}
		}
	}

	sort.SliceStable(td.Partitions, func(i, j int) bool {
		return td.Partitions[i].ID < td.Partitions[j].ID
	})

	d := g.NewGoFileDestination(outputFilename)

	if err := d.WriteTemplate("endpoints", tmpl, td, templateFuncMap); err != nil {
		g.Fatalf("error generating endpoint resolver: %s", err)
	}

	if err := d.Write(); err != nil {
		g.Fatalf("generating file (%s): %s", outputFilename, err)
	}
}

func readHTTPJSON(url string, to any) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return decodeFromReader(r.Body, to)
}

func decodeFromReader(r io.Reader, to any) error {
	dec := json.NewDecoder(r)

	for {
		if err := dec.Decode(to); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}

	return nil
}

//go:embed output.go.gtpl
var tmpl string
