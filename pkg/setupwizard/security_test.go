package setupwizard

import (
	"strings"
	"testing"
)

func TestQuoteShellArgumentPreventsCommandSubstitution(t *testing.T) {
	quoted, err := quoteShellArgument("/tmp/$(touch pwned)'binary")
	if err != nil {
		t.Fatal(err)
	}
	if quoted != `'/tmp/$(touch pwned)'\''binary'` {
		t.Fatalf("quoted argument = %q", quoted)
	}
}

func TestQuoteShellArgumentRejectsNewline(t *testing.T) {
	if _, err := quoteShellArgument("safe\ncommand"); err == nil {
		t.Fatal("quoteShellArgument accepted a newline")
	}
}

func TestQuoteSystemdArgumentRejectsDirectiveInjection(t *testing.T) {
	if _, err := quoteSystemdArgument("/srv/app\nExecStart=/bin/false"); err == nil {
		t.Fatal("quoteSystemdArgument accepted a newline")
	}
}

func TestQuoteSystemdArgumentEscapesQuotesAndBackslashes(t *testing.T) {
	quoted, err := quoteSystemdArgument(`/srv/a b/"quoted"`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(quoted, `"`) || !strings.HasSuffix(quoted, `"`) {
		t.Fatalf("quoted argument = %q", quoted)
	}
	if strings.Contains(quoted[1:len(quoted)-1], `"quoted"`) {
		t.Fatalf("inner quotes were not escaped: %q", quoted)
	}
}
