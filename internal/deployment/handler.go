package deployment

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"regexp"
	"time"

	fdk "github.com/fnproject/fdk-go"
)

var attemptPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Handler dispatches only deployment-owned operations. Invocation data cannot
// change trusted configuration or grant notification-delivery authority.
type Handler struct {
	getenv  func(string) string
	connect func(Settings) (Store, error)
	compose func(Settings, Store, pinnedInputs, string) (operations, error)
	metrics func(Settings, Store, string) (metricPublisher, error)
	logger  *log.Logger
}

func New(getenv func(string) string) *Handler {
	return &Handler{getenv: getenv, connect: connect, compose: composeOperations, metrics: composeMetrics, logger: log.New(os.Stderr, "", 0)}
}

func (h *Handler) Serve(ctx context.Context, in io.Reader, out io.Writer) {
	result := h.run(ctx, in)
	h.logResult(result)
	body, success := result.output()
	fdk.SetHeader(out, "Content-Type", "application/json")
	if !success {
		fdk.WriteStatus(out, 400)
	}
	// FDK's transport status alone does not signify success. RM checks this body.
	_ = json.NewEncoder(out).Encode(body)
}

func (h *Handler) run(ctx context.Context, in io.Reader) (result response) {
	started := time.Now()
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return rejected("deadline")
	}
	original := ctx
	ctx, cancel := context.WithDeadline(ctx, started.Add(20*time.Second))
	defer cancel()
	fnctx := fdk.GetContext(ctx)
	request, inputFailure := readCommand(in, fnctx.Header())
	if inputFailure != "" {
		result = rejected("request_invalid")
		result.inputRejection = describeInputRejection(fnctx, inputFailure)
		return result
	}
	if ctx.Err() != nil {
		return rejected("deadline")
	}
	// Correlate every later outcome without reflecting malformed request data.
	defer func() { result.legacy.CheckAttemptID = request.attemptID }()
	settings, err := parseSettings(h.getenv("ROUTER_BOOTSTRAP"))
	if err != nil {
		return rejected("bootstrap_invalid")
	}
	if (request.operation == "deployment-check" || request.operation == "close-work" || request.operation == "recover-work") && (request.configurationSHA256 != settings.ConfigurationSHA256 || request.releaseSHA256 != settings.ReleaseSHA256) {
		return rejected("request_mismatch")
	}
	store, err := h.connect(settings)
	if err != nil || store == nil {
		return rejected("authentication")
	}
	p, failure := loadPinned(ctx, settings, store)
	if failure.Status != "" {
		return response{legacy: failure}
	}
	if request.intakeTopic != "" && request.intakeTopic != p.config.IntakeTopicID {
		return rejected("intake_mismatch")
	}
	if request.operation == "close-work" && !matchesClosure(p.config, request) {
		return rejected("closure_intent_mismatch")
	}
	if request.operation == "recover-work" && !matchesSendingRecovery(p.config, request) {
		return rejected("recovery_intent_mismatch")
	}
	if request.operation == "review-recovery" && p.config.Mode != "LIVE" {
		return rejected("recovery_review_requires_live")
	}
	if p.config.SchemaVersion == 7 {
		// Delayed automatic requests must not prepare a baseline, execute saved
		// recovery or run tests while the installation is not LIVE.
		if request.automatic && p.config.Mode != "LIVE" {
			idle := initialOperationalResult(settings, p, request)
			idle.Status = "idle"
			return response{operational: &idle}
		}
		var publisher metricPublisher
		if p.config.Mode == "LIVE" && (liveProcessingOperation(request.operation) || request.operation == "deployment-check") {
			publisher, err = h.metrics(settings, store, p.config.InstallationID)
			if err != nil || publisher == nil {
				failed := initialOperationalResult(settings, p, request)
				failed.Code = "monitoring_configuration"
				return response{operational: &failed}
			}
		}
		operational := h.runOperational(original, started, settings, store, p, request)
		if publisher != nil && liveProcessingOperation(request.operation) {
			publishOperationalMetrics(original, publisher, &operational)
		}
		return response{operational: &operational}
	}
	if request.operation != "deployment-check" {
		return rejected("configuration_unsupported")
	}
	return response{legacy: checkLegacy(ctx, settings, store, request.attemptID, p)}
}
