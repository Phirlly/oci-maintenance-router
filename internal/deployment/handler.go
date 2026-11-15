package deployment

import (
	"context"
	"encoding/json"
	"io"
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
}

func New(getenv func(string) string) *Handler {
	return &Handler{getenv: getenv, connect: connect, compose: composeOperations}
}

func (h *Handler) Serve(ctx context.Context, in io.Reader, out io.Writer) {
	result := h.run(ctx, in)
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
	body, err := io.ReadAll(io.LimitReader(in, 8193))
	if err != nil || len(body) > 8192 {
		return rejected("request_invalid")
	}
	if ctx.Err() != nil {
		return rejected("deadline")
	}
	request, err := parseCommand(body)
	if err != nil {
		return rejected("request_invalid")
	}
	// Correlate every later outcome without reflecting malformed request data.
	defer func() { result.legacy.CheckAttemptID = request.attemptID }()
	settings, err := parseSettings(h.getenv("ROUTER_BOOTSTRAP"))
	if err != nil {
		return rejected("bootstrap_invalid")
	}
	if request.operation == "deployment-check" && (request.configurationSHA256 != settings.ConfigurationSHA256 || request.releaseSHA256 != settings.ReleaseSHA256) {
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
	if p.config.SchemaVersion == 7 {
		operational := h.runOperational(original, started, settings, store, p, request)
		return response{operational: &operational}
	}
	if request.operation != "deployment-check" {
		return rejected("configuration_unsupported")
	}
	return response{legacy: checkLegacy(ctx, settings, store, request.attemptID, p)}
}
