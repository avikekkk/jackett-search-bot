package bot

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		text        string
		wantCommand string
		wantArg     string
		wantPayload string
		wantOK      bool
	}{
		{"/r ubuntu server", "r", "ubuntu", "ubuntu server", true},
		{"/r@MyJackettBot ubuntu", "r", "ubuntu", "ubuntu", true},
		{"/START", "start", "", "", true},
		{"  /help  ", "help", "", "", true},
		{"hello", "", "", "", false},
		{"/", "", "", "", false},
		{"/@MyJackettBot", "", "", "", false},
		{"/r\nMarvel's Spider-Man 2", "r", "Marvel's", "Marvel's Spider-Man 2", true},
		// Mention and case are both normalized away.
		{"/R@MyJackettBot ubuntu", "r", "ubuntu", "ubuntu", true},
		// The mention is matched case-insensitively, as Telegram treats it.
		{"/r@myjackettbot ubuntu", "r", "ubuntu", "ubuntu", true},
		// Addressed to a different bot in the same group: not ours to answer.
		{"/help@SomeOtherBot", "", "", "", false},
		{"/auth@cosmosusenetbot 123", "", "", "", false},
		{"/logs@MyCloneBot", "", "", "", false},
	}

	b := &Bot{username: "MyJackettBot"}

	for _, tc := range cases {
		command, args, payload, ok := b.parseCommand(tc.text)
		if ok != tc.wantOK {
			t.Errorf("parseCommand(%q) ok = %v, want %v", tc.text, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if command != tc.wantCommand {
			t.Errorf("parseCommand(%q) command = %q, want %q", tc.text, command, tc.wantCommand)
		}
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		if arg != tc.wantArg {
			t.Errorf("parseCommand(%q) first arg = %q, want %q", tc.text, arg, tc.wantArg)
		}
		if payload != tc.wantPayload {
			t.Errorf("parseCommand(%q) payload = %q, want %q", tc.text, payload, tc.wantPayload)
		}
	}
}
