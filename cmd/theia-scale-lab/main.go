package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/lollinoo/theia/internal/scalelab"
)

func main() {
	profileFlag := flag.String("profile", "100", "Built-in profile: 100, 300, 500, or 1000")
	scenarioFlag := flag.String("scenario", "baseline", "Built-in scenario: baseline, db-slowdown, snmp-timeout-spike, burst-adds, burst-unresolved-neighbors, soak-24h")
	fixtureFlag := flag.String("fixture", "", "Optional replay fixture JSON path")
	outFlag := flag.String("out", "", "Optional output file path for the JSON report")
	flag.Parse()

	profile, err := scalelab.BuiltinProfile(*profileFlag)
	if err != nil {
		log.Fatalf("Invalid profile: %v", err)
	}

	scenario, err := scalelab.BuiltinScenario(*scenarioFlag, profile)
	if err != nil {
		log.Fatalf("Invalid scenario: %v", err)
	}

	fixture := scalelab.GenerateSyntheticFixture(profile, scenario)
	if *fixtureFlag != "" {
		raw, err := os.ReadFile(*fixtureFlag)
		if err != nil {
			log.Fatalf("Failed to read fixture: %v", err)
		}
		fixture = scalelab.ReplayFixture{}
		if err := json.Unmarshal(raw, &fixture); err != nil {
			log.Fatalf("Failed to decode fixture: %v", err)
		}
	}

	report, err := scalelab.Run(profile, scenario, fixture)
	if err != nil {
		log.Fatalf("Scale lab failed: %v", err)
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("Failed to encode report: %v", err)
	}

	if *outFlag != "" {
		if err := os.WriteFile(*outFlag, payload, 0644); err != nil {
			log.Fatalf("Failed to write report: %v", err)
		}
	}

	os.Stdout.Write(payload)
	os.Stdout.Write([]byte("\n"))
}
