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

const Version = "0.1.3"

const cursorSchema = "v1"

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

	var segmentWriteKey, bugoutToken, bugoutJournalID, cursorName, cursor string
	var batchSize, timeout int
	var checkVersion, debug bool
	flag.StringVar(&segmentWriteKey, "segment", "", "Segment write key (get one by creating a source at https://segment.com)")
	flag.StringVar(&bugoutToken, "token", "", "Bugout access token (create one at https://bugout.dev/account/tokens)")
	flag.StringVar(&bugoutJournalID, "journal", "", "Bugout journal ID to load events from")
	flag.StringVar(&cursorName, "cursorname", "", "(Optional) Name of cursor to use for the job - this is the name under which cursor gets stored in journal")
	flag.StringVar(&cursor, "cursor", "", "(Optional) Cursor to use for job (if specified, we do not retrieve state of cursor from journal)")
	flag.IntVar(&batchSize, "N", 100, "Number of reports to process per iteration")
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

	if cursorName != "" && cursor == "" {
		fmt.Printf("Loading cursor (cursorname=%s) from journal...\n", cursorName)
		journalCursor, cursorErr := getCursorFromJournal(bugoutClient, bugoutToken, bugoutJournalID, cursorName)
		if cursorErr != nil {
			panic(fmt.Errorf("Error retrieving cursor (cursorname=%s) from Bugout:\n%s\n", cursorName, err.Error()))
		}
		cursor = journalCursor
		fmt.Printf("Loaded cursor with cursorname=%s: %s", cursorName, cursor)
	}

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
		if cursorName != "" && newCursor != "" {
			fmt.Printf("Writing cursor (%s) to journal with cursorname=%s\n", newCursor, cursorName)
			writeCursorErr := writeCursorToJournal(bugoutClient, bugoutToken, bugoutJournalID, cursorName, newCursor)
			if writeCursorErr != nil {
				fmt.Printf("[WARNING] Could not write cursor (cursorname=%s) to Bugout:\n%s\n", cursorName, writeCursorErr.Error())
			}
		}
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

func getCursorFromJournal(client bugout.BugoutClient, token, journalID, cursorName string) (string, error) {
	query := fmt.Sprintf("context_type:thorax tag:type:cursor tag:cursor_schema:%s tag:cursor:%s", cursorSchema, cursorName)
	parameters := map[string]string{
		"order":   "desc",
		"content": "true", // We may use the content in the future, even though we are simply using context_url right now
	}
	results, err := client.Spire.SearchEntries(token, journalID, query, 1, 0, parameters)
	if err != nil {
		return "", err
	}

	if results.TotalResults == 0 {
		return "", nil
	}

	return results.Results[0].ContextUrl, nil
}

func writeCursorToJournal(client bugout.BugoutClient, token, journalID, cursorName, cursor string) error {
	title := fmt.Sprintf("thorax cursor: %s", cursorName)
	entryContext := spire.EntryContext{
		ContextType: "thorax",
		ContextID:   cursor,
		ContextURL:  cursor,
	}
	tags := []string{
		"type:cursor",
		fmt.Sprintf("cursor_schema:%s", cursorSchema),
		fmt.Sprintf("cursor:%s", cursorName),
		fmt.Sprintf("thorax_version:%s", Version),
	}
	_, err := client.Spire.CreateEntry(token, journalID, title, cursor, tags, entryContext)
	return err
}

func reportsIterator(client bugout.BugoutClient, token, journalID, cursor string, limit, offset int) (spire.EntryResultsPage, error) {
	var query string = "!tag:type:cursor"
	if cursor != "" {
		cleanedCursor := cleanTimestamp(cursor)
		query = fmt.Sprintf("%s created_at:>%s", query, cleanedCursor)
		fmt.Println("query:", query)
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
		link := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(entry.Url, "http://", "https://"), "spire.bugout.dev", "bugout.dev"), "/journals/", "/app/personal/")
		entryProperties.Set("link", link)
		clientID := "unknown"
		username := ""
		for _, tag := range entry.Tags {
			components := strings.SplitN(tag, ":", 2)
			if len(components) < 2 {
				entryProperties.Set(tag, true)
			} else {
				entryProperties.Set(components[0], components[1])
				if components[0] == "client" {
					clientID = components[1]
				} else if components[0] == "username" {
					username = components[1]
				}
			}
		}

		if clientID != "unknown" && username != "" {
			identificationErr := client.Enqueue(analytics.Identify{
				UserId: clientID,
				Traits: analytics.NewTraits().SetUsername(username),
			})
			if identificationErr != nil {
				panic(identificationErr)
			}
		}

		cleanedCreatedAt := cleanTimestamp(entry.CreatedAt)
		timestamp, timestampParseErr := time.Parse(time.RFC3339, cleanedCreatedAt)
		if timestampParseErr != nil {
			fmt.Printf("WARNING: Could not parse time (%s) using layout string (%s)\n", cleanedCreatedAt, time.RFC3339)
		}

		err := client.Enqueue(analytics.Track{
			Event:      entry.Title,
			UserId:     clientID,
			Properties: entryProperties,
			Timestamp:  timestamp,
		})
		if err != nil {
			panic(err)
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return cleanTimestamp(entries[len(entries)-1].CreatedAt)
}
