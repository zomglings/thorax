package thorax

import (
	"os"

	humbug "github.com/bugout-dev/humbug/go/pkg"
	"github.com/google/uuid"
)

var thoraxReporterToken string = "357c7247-5f6e-4f16-83f1-6ae95dadc6ff"

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
}
