package policy

import "testing"

func TestSSHReadOnlyConservativeGrammar(t *testing.T) {
	allowed := []string{"free -m", "ls -lah /srv", "cat '/var/log/app log' | tail -n 20", "grep -F 'error' /var/log/app.log && wc -l /var/log/app.log", "journalctl -u app --no-pager -n 40", "systemctl status app --no-pager", "uname -a"}
	for _, command := range allowed {
		if !SSHReadOnly(command, false, nil) {
			t.Errorf("rejected read %q", command)
		}
	}
	denied := []string{"", "echo test", "touch /tmp/a", "cat /tmp/a > /tmp/b", "cat /tmp/a 2>&1", "cat $(touch /tmp/a)", "cat `touch /tmp/a`", "A=1 cat /tmp/a", "env cat /tmp/a", "/tmp/cat /tmp/a", "sudo cat /tmp/a", "sh -c 'cat /tmp/a'", "python3 -c 'print(1)'", "./check.sh", "cat /tmp/*", "cat \"$FILE\"", "cat <(touch /tmp/a)", "ls &", "for x in a; do cat a; done", "ls; touch /tmp/a", "if true; then ls; fi", "hostname evil", "journalctl --rotate --no-pager", "journalctl --vacuum-time=1s --no-pager", "systemctl restart app --no-pager", "systemctl status app", "cat 'unterminated"}
	for _, command := range denied {
		if SSHReadOnly(command, false, nil) {
			t.Errorf("allowed unknown/write %q", command)
		}
	}
	if SSHReadOnly("free -m", true, nil) || SSHReadOnly("free -m", false, map[string]string{"PATH": "/tmp"}) {
		t.Fatal("privilege/environment bypass")
	}
}

func TestScriptsAreTierAwareButDestructiveHardStopsRemain(t *testing.T) {
	for _, command := range []string{"python3 -c 'print(1)'", "bash /srv/check.sh", "sh /srv/check.sh"} {
		r := EvaluateSSH(SSHRequest{Command: command, AllowScripts: true})
		if r.Decision != DecisionAllowed {
			t.Fatalf("script %q: %+v", command, r)
		}
	}
	for _, command := range []string{"rm -rf /usr", "mkfs.ext4 /dev/sda", "dd if=/tmp/image of=/dev/sda", "sh -c 'rm -rf /usr'"} {
		r := EvaluateSSH(SSHRequest{Command: command, AllowScripts: true})
		if r.Decision != DecisionPermanentlyRejected {
			t.Fatalf("hard stop lost for %q: %+v", command, r)
		}
	}
}
