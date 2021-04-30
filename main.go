package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	bugout "github.com/bugout-dev/bugout-go/pkg"
	spire "github.com/bugout-dev/bugout-go/pkg/spire"
	humbug "github.com/bugout-dev/humbug/go/pkg"
	"github.com/google/uuid"
	"github.com/segmentio/analytics-go"
)

var thoraxReporterToken string = "357c7247-5f6e-4f16-83f1-6ae95dadc6ff"

const Version = "0.0.2"

func main() {
	consent := humbug.CreateHumbugConsent(humbug.EnvironmentVariableConsent("THORAX_REPORTING_ENABLED", humbug.Yes, false))
	clientID := os.Getenv("THORAX_EMAIL")
	sessionID := uuid.NewString()
	reporter, err := humbug.CreateHumbugReporter(consent, clientID, sessionID, thoraxReporterToken)
	if err != nil {
		panic(err)
	}

	defer func() {
		message := recover()
		if message != nil {
			report := humbug.PanicReport(message)
			reporter.Publish(report)
			panic(message)
		}
	}()

	report := humbug.SystemReport()
	reporter.Publish(report)

	var segmentWriteKey, bugoutToken, bugoutJournalID, cursor string
	var batchSize, timeout int
	var checkVersion, debug bool
	flag.StringVar(&segmentWriteKey, "segment", "", "Segment write key (get one by creating a source at https://segment.com)")
	flag.StringVar(&bugoutToken, "token", "", "Bugout access token (create one at https://bugout.dev/account/tokens)")
	flag.StringVar(&bugoutJournalID, "journal", "", "Bugout journal ID to load events from")
	flag.StringVar(&cursor, "cursor", "", "(Optional) cursor from which to start loading items")
	flag.IntVar(&batchSize, "N", 1000, "Number of reports to process per iteration")
	flag.IntVar(&timeout, "s", 0, "Number of seconds to wait between Bugout requests")
	flag.BoolVar(&debug, "debug", false, "Set this to true to run in debug mode")
	flag.BoolVar(&checkVersion, "version", false, "Run with this flag to see the current Thorax version and immediately exit")
	flag.Parse()

	if checkVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if segmentWriteKey == "" {
		panic("Please pass a segment write key using the -segment option.")
	}

	if bugoutToken == "" {
		panic("Please pass a Bugout token using the -token option.")
	}

	if bugoutJournalID == "" {
		panic("Please pass a Bugout journal ID using the -journal option.")
	}

	if debug {
		fmt.Printf("DEBUG: Bugout access token: %s\n", bugoutToken)
		fmt.Printf("DEBUG: Bugout journal ID: %s\n", bugoutJournalID)
		fmt.Printf("DEBUG: Segment write key: %s\n", segmentWriteKey)
		fmt.Printf("DEBUG: Cursor: %s\n", cursor)
		fmt.Printf("DEBUG: Batch size: %d\n", batchSize)
		fmt.Printf("DEBUG: Timeout: %d\n", timeout)
	}

	bugoutClient, err := bugout.ClientFromEnv()
	if err != nil {
		panic(err)
	}

	segmentClient, err := analytics.NewWithConfig(segmentWriteKey, analytics.Config{
		Interval:  1 * time.Second,
		BatchSize: 100,
	})
	if err != nil {
		panic(err)
	}
	defer segmentClient.Close()

	newCursor := cursor
	halt := false
	offset := 0
	for {
		results, err := reportsIterator(bugoutClient, bugoutToken, bugoutJournalID, cursor, batchSize, offset)
		if err != nil {
			panic(err)
		}
		loadingUntil := offset + batchSize
		if results.TotalResults < loadingUntil {
			loadingUntil = results.TotalResults
		}
		fmt.Printf("Loading to Segment: %d to %d of %d\n", offset, loadingUntil, results.TotalResults)
		offset = results.NextOffset
		if results.NextOffset == 0 {
			halt = true
		}
		newCursor = loadToSegment(segmentClient, results.Results)
		fmt.Printf("New cursor: %s\n", newCursor)
		if halt {
			break
		} else {
			time.Sleep(time.Duration(timeout) * time.Second)
		}
	}
	fmt.Println("Done!")
}

func cleanTimestamp(rawTimestamp string) string {
	return strings.ReplaceAll(rawTimestamp, " ", "T")
}

func reportsIterator(client bugout.BugoutClient, token, journalID, cursor string, limit, offset int) (spire.EntryResultsPage, error) {
	var query string = ""
	if cursor != "" {
		cleanedCursor := cleanTimestamp(cursor)
		query = fmt.Sprintf("created_at:>%s", cleanedCursor)
	}
	parameters := map[string]string{
		"order":   "asc",
		"content": "false",
	}
	return client.Spire.SearchEntries(token, journalID, query, limit, offset, parameters)
}

func loadToSegment(client analytics.Client, entries []spire.Entry) string {
	for _, entry := range entries {
		entryProperties := analytics.NewProperties()
		sessionID := "unknown"
		for _, tag := range entry.Tags {
			components := strings.SplitN(tag, ":", 2)
			if len(components) < 2 {
				entryProperties.Set(tag, true)
			} else {
				entryProperties.Set(components[0], components[1])
				if components[0] == "session" {
					sessionID = components[1]
				}
			}
		}

		err := client.Enqueue(analytics.Track{
			Event:      entry.Title,
			UserId:     sessionID,
			Properties: entryProperties,
		})
		if err != nil {
			panic(err)
		}
	}
	return cleanTimestamp(entries[len(entries)-1].CreatedAt)
}
