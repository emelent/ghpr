package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// stubExec makes Ask run script with sh instead of claude, recording the
// arguments; the script sees the prompt on stdin and $STDIN_FILE is where
// it should save it.
func stubExec(t *testing.T, script string) (args *[]string, stdin func() string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "stdin")
	got := new([]string)
	execCommand = func(name string, a ...string) *exec.Cmd {
		*got = append([]string{name}, a...)
		cmd := exec.Command("sh", "-c", "cat > \"$STDIN_FILE\"; "+script)
		cmd.Env = append(os.Environ(), "STDIN_FILE="+file)
		return cmd
	}
	t.Cleanup(func() { execCommand = exec.Command })
	return got, func() string { b, _ := os.ReadFile(file); return string(b) }
}

func TestArgs(t *testing.T) {
	base := []string{"-p", "--output-format", "json", "--tools", "", "--append-system-prompt", SystemPrompt}
	if got := Args("", "", false); !reflect.DeepEqual(got, base) {
		t.Fatalf("without a checkout every tool should be off: %v", got)
	}
	want := append(append([]string{}, base...), "--model", "opus", "--resume", "s1")
	want[4] = Tools
	if got := Args("opus", "s1", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("Args(opus, s1, true) = %v", got)
	}
}

func TestAsk(t *testing.T) {
	args, stdin := stubExec(t, `printf '%s' '{"type":"result","is_error":false,"result":"  It prints hi.\n","session_id":"abc"}'`)
	a, err := Ask("", "", "", "", "what does this do?")
	if err != nil {
		t.Fatal(err)
	}
	if a.Text != "It prints hi." || a.SessionID != "abc" {
		t.Fatalf("answer = %+v", a)
	}
	if (*args)[0] != DefaultBin {
		t.Fatalf("empty bin should run %q, ran %q", DefaultBin, (*args)[0])
	}
	if stdin() != "what does this do?" {
		t.Fatalf("prompt should go on stdin, got %q", stdin())
	}
	// With a checkout, claude runs there with the read-only tools.
	dir := t.TempDir()
	args, _ = stubExec(t, `printf '{"result":"%s","session_id":"abc"}' "$(pwd)"`)
	a, err = Ask("/opt/claude", "sonnet", "abc", dir, "and then?")
	if err != nil {
		t.Fatal(err)
	}
	if want := append([]string{"/opt/claude"}, Args("sonnet", "abc", true)...); !reflect.DeepEqual(*args, want) {
		t.Fatalf("args = %v, want %v", *args, want)
	}
	if filepath.Base(a.Text) != filepath.Base(dir) {
		t.Fatalf("claude should run in the checkout %s, ran in %q", dir, a.Text)
	}
}

func TestAskErrors(t *testing.T) {
	stubExec(t, `printf '%s' '{"is_error":true,"result":"Credit balance is too low"}'`)
	if _, err := Ask("", "", "", "", "q"); err == nil || err.Error() != "Credit balance is too low" {
		t.Fatalf("is_error should surface the result: %v", err)
	}
	stubExec(t, `echo 'Invalid API key' >&2; exit 1`)
	if _, err := Ask("", "", "", "", "q"); err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("a failed run should surface stderr: %v", err)
	}
	stubExec(t, `echo 'not json'`)
	if _, err := Ask("", "", "", "", "q"); err == nil || !strings.Contains(err.Error(), "unreadable output") {
		t.Fatalf("garbage output should be an error: %v", err)
	}
}

func TestAvailable(t *testing.T) {
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = exec.LookPath })
	if Available("claude") {
		t.Fatal("missing binary should not be available")
	}
	lookPath = func(string) (string, error) { return "/bin/claude", nil }
	if !Available("claude") {
		t.Fatal("found binary should be available")
	}
}
