package supervisor

import "testing"

func TestExtractLogLevel(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		want  string
	}{
		{
			name: "spring boot INFO with error-prefixed class names should be INFO",
			line: "2026-08-18 16:28:47.665 INFO 476820 --- [ main] s.w.s.m.m.a.RequestMappingHandlerMapping : Mapped \"{[/error],produces=[text/html]}\" onto public org.springframework.web.servlet.ModelAndView org.springframework.boot.autoconfigure.web.BasicErrorController.errorHtml(javax.servlet.http.HttpServletRequest,javax.servlet.http.HttpServletResponse)",
			want: "INFO",
		},
		{
			name: "spring boot INFO with BasicErrorController.error should be INFO",
			line: "2026-08-18 16:28:47.666 INFO 476820 --- [ main] s.w.s.m.m.a.RequestMappingHandlerMapping : Mapped \"{[/error]}\" onto public org.springframework.http.ResponseEntity<java.util.Map<java.lang.String, java.lang.Object>> org.springframework.boot.autoconfigure.web.BasicErrorController.error(javax.servlet.http.HttpServletRequest)",
			want: "INFO",
		},
		{
			name: "bracketed INFO",
			line: "2026-08-18 10:00:00 [INFO] application started",
			want: "INFO",
		},
		{
			name: "bracketed ERROR",
			line: "2026-08-18 10:00:00 [ERROR] boom",
			want: "ERROR",
		},
		{
			name: "explicit ERROR token",
			line: "2026-08-18 10:00:00 ERROR something failed",
			want: "ERROR",
		},
		{
			name: "Level=ERROR key-value",
			line: "ts=... Level=ERROR msg=foo",
			want: "ERROR",
		},
		{
			name: "generic error() call should not be ERROR and should fallback INFO",
			line: "2026-08-18 10:00:00 calling error() and handleError() routine finished ok",
			want: "INFO",
		},
		{
			name: "DEBUG token",
			line: "2026-08-18 10:00:00 DEBUG start loop",
			want: "DEBUG",
		},
		{
			name: "WARN normalized to WARNING",
			line: "2026-08-18 10:00:00 WARN low disk",
			want: "WARNING",
		},
		{
			name: "TRACE token",
			line: "2026-08-18 10:00:00 TRACE detail line",
			want: "TRACE",
		},
		{
			name: "plain message default INFO",
			line: "just a plain message",
			want: "INFO",
		},
		{
			name: "errorCode field value should not trigger ERROR unless boundary",
			line: "2026-08-18 10:00:00 INFO errorCode=200 processed",
			want: "INFO",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLogLevel(tc.line)
			if got != tc.want {
				t.Errorf("extractLogLevel(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}
func TestParseLogEntriesLevels(t *testing.T) {
	// 模拟真实 Spring Boot 日志文件片段(含标题行/INFO/ERROR 混合)
	logText := `2026-08-18 16:28:47.665  INFO 476820 --- [main] s.w.s.m.m.a.RequestMappingHandlerMapping : Mapped "{[/error],produces=[text/html]}"
2026-08-18 16:28:47.666  INFO 476820 --- [main] s.w.s.m.m.a.RequestMappingHandlerMapping : Mapped "{[/error]}"
2026-08-18 16:28:47.999 ERROR 476820 --- [main] com.foo.Bar : NullPointerException at org.springframework.boot.autoconfigure.web.BasicErrorController.error()
2026-08-18 16:28:48.100  WARN 476820 --- [main] com.foo.Baz : low disk space
2026-08-18 16:28:48.200 DEBUG 476820 --- [main] com.foo.Debug : entering loop`

	n := &Node{Name: "node-11"}
	entries := n.parseLogEntries(logText, "stdout", "iptv-feedback-fail")

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	want := []string{"INFO", "INFO", "ERROR", "WARNING", "DEBUG"}
	for i, w := range want {
		if entries[i].Level != w {
			t.Errorf("entry %d level = %q, want %q (msg=%q)", i, entries[i].Level, w, entries[i].Message[:60])
		}
	}
}
