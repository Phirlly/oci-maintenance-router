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

// Handler accepts only a nonsending installation check. Invocation data cannot
// change the trusted configuration or enable notification delivery.
type Handler struct {
	getenv  func(string) string
	connect func(Settings) (Store, error)
}

func New(getenv func(string) string) *Handler {
	return &Handler{getenv: getenv, connect: connect}
}

func (h *Handler) Serve(ctx context.Context, in io.Reader, out io.Writer) {
	result := h.run(ctx, in)
	fdk.SetHeader(out, "Content-Type", "application/json")
	if result.Status != "deployment-verified" {
		fdk.WriteStatus(out, 400)
	}
	// FDK's transport status alone does not signify success. RM checks this body.
	_ = json.NewEncoder(out).Encode(result)
}

func (h *Handler) run(ctx context.Context, in io.Reader) (result Result) {
	if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
		return failed("deadline")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	body, err := io.ReadAll(io.LimitReader(in, 1025))
	if err != nil || len(body) > 1024 {
		return failed("request_invalid")
	}
	if ctx.Err() != nil {
		return failed("deadline")
	}
	var request struct {
		Operation           string `json:"operation"`
		ConfigurationSHA256 string `json:"configurationSha256"`
		ReleaseSHA256       string `json:"releaseSha256"`
		CheckAttemptID      string `json:"checkAttemptId"`
	}
	if decodeJSON(body, &request) != nil || request.Operation != "deployment-check" || !attemptPattern.MatchString(request.CheckAttemptID) {
		return failed("request_invalid")
	}
	// Correlate every later outcome without reflecting malformed request data.
	defer func() { result.CheckAttemptID = request.CheckAttemptID }()
	settings, err := parseSettings(h.getenv("ROUTER_BOOTSTRAP"))
	if err != nil {
		return failed("bootstrap_invalid")
	}
	if request.ConfigurationSHA256 != settings.ConfigurationSHA256 || request.ReleaseSHA256 != settings.ReleaseSHA256 {
		return failed("request_mismatch")
	}
	store, err := h.connect(settings)
	if err != nil {
		return failed("authentication")
	}
	return check(ctx, settings, store, request.CheckAttemptID)
}
