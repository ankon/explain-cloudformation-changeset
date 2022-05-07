package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/ankon/explain-cloudformation-changeset/util"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go/logging"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	log "github.com/sirupsen/logrus"
)

const defaultLayoutName = string(graphviz.DOT)

func getEnvOrDefault(defaultValue string, names ...string) string {
	for _, name := range names {
		val, present := os.LookupEnv(name)
		if present {
			return val
		}
	}

	return defaultValue
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("cannot determine current working directory (%q)", err.Error())
	}

	stackName := flag.String("stack-name", "", "Root stack name (required when change set is not given as ARN)")
	changeSetName := flag.String("change-set-name", "", "Root change set name")
	cacheDir := flag.String("cache-dir", cwd, "Directory for caching changeset descriptions")
	graphFile := flag.String("graph-output", "", "File to write changeset graph (should be using .dot/.svg/.png/.jpg extension")
	region := flag.String("region", getEnvOrDefault("us-east-1", "AWS_REGION", "AWS_DEFAULT_REGION"), "AWS region")
	var layoutName string
	flag.StringVar(&layoutName, "layout", defaultLayoutName, "Graphviz layout engine")
	flag.StringVar(&layoutName, "K", defaultLayoutName, "Graphviz layout engine")
	flag.Parse()

	if *changeSetName == "" {
		flag.PrintDefaults()
		log.Fatalf("must provide change set name")
	}

	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	awsLogger := logging.LoggerFunc(func(classification logging.Classification, format string, v ...interface{}) {
		log.WithField("process", "s3").Debug(v...)
	})
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(*region),
		config.WithLogger(awsLogger))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	// Using the Config value, create the DynamoDB client
	svc, err := util.NewClientWithCache(cloudformation.NewFromConfig(cfg), &util.ClientWithCacheOpts{CacheDir: cacheDir})
	if err != nil {
		log.Fatalf("cannot create client, %v", err)
	}

	if *graphFile != "" {
		g := graphviz.New()
		graph, err := g.Graph(
			graphviz.Directed,
			graphviz.Name(*changeSetName),
		)
		if err != nil {
			log.Fatalf("failed to create new graph, %v", err)
		}
		defer func() {
			if err := graph.Close(); err != nil {
				// XXX: Can we somehow return this error, rather than panicing?
				log.Fatal(err)
			}
			g.Close()
		}()

		layout := graphviz.Layout(layoutName)
		g.SetLayout(layout)
		if layout == graphviz.SFDP || layout == graphviz.FDP {
			// See https://gitlab.com/graphviz/graphviz/-/issues/1269, the go-graphviz library
			// doesn't have triangulation either ("delaunay_tri: Graphviz built without any triangulation library")
			// XXX: This should accept a string, not a boolean!
			graph.SetOverlap(true)
		}

		_, err = util.NewChangeSetGraph(graph, svc, *stackName, *changeSetName)
		if err != nil {
			log.Fatalf("unable to build graph, %v", err)
		}

		var format graphviz.Format
		ext := strings.ToLower(filepath.Ext(*graphFile))
		switch ext {
		case ".png":
			format = graphviz.PNG
		case ".jpg":
			fallthrough
		case ".jpeg":
			format = graphviz.JPG
		case ".svg":
			format = graphviz.SVG
		case ".dot":
			format = graphviz.XDOT
		default:
			format = graphviz.PNG
		}

		graph.SetRankDir(cgraph.LRRank)

		var buf bytes.Buffer
		if err := g.Render(graph, format, &buf); err != nil {
			log.Fatal(err)
		}
		err = os.WriteFile(*graphFile, buf.Bytes(), 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
}
