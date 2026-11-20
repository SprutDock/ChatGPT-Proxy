package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sprutdock/cert-agent/internal/state"
)

func InstallBackgroundCheck(p state.Paths, interactive bool) error {
	stunnelAgent := StunnelAgentPath(p)
	schedulerAgent := SchedulerAgentPath(p)
	if err := state.EnsurePrivateDir(filepath.Dir(stunnelAgent)); err != nil {
		return err
	}
	stunnel := filepath.Join(p.Resources, "stunnel")
	if _, err := os.Stat(stunnel); err != nil {
		return fmt.Errorf("bundled stunnel is unavailable")
	}
	uid, err := CurrentUID()
	if err != nil {
		return err
	}
	domain := "gui/" + uid
	stunnelPlist := launchAgentPlist(StunnelLabel, []string{stunnel, p.StunnelConf}, p.StunnelLog, true, 0)
	if err := state.WriteAtomic(stunnelAgent, []byte(stunnelPlist), 0o600); err != nil {
		return err
	}
	agent := filepath.Join(p.Resources, "cert-agent")
	schedulerPlist := launchAgentPlist(AgentLabel, []string{agent, "ensure", "--background"}, p.Log, false, 12*60*60)
	if !interactive && available(schedulerAgent) {
		return ensureJobLoaded(domain, AgentLabel, schedulerAgent)
	}
	changed, err := state.WriteIfChanged(schedulerAgent, []byte(schedulerPlist), 0o600)
	if err != nil {
		return err
	}
	if changed {
		return reloadJob(domain, AgentLabel, schedulerAgent)
	}
	return ensureJobLoaded(domain, AgentLabel, schedulerAgent)
}

func launchAgentPlist(label string, arguments []string, logPath string, keepAlive bool, interval int) string {
	var values strings.Builder
	for _, argument := range arguments {
		values.WriteString("<string>" + plistEscape(argument) + "</string>")
	}
	var schedule strings.Builder
	if keepAlive {
		schedule.WriteString("<key>KeepAlive</key><true/><key>RunAtLoad</key><true/>")
	}
	if interval > 0 {
		schedule.WriteString("<key>StartInterval</key><integer>" + strconv.Itoa(interval) + "</integer>")
	}
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\"><plist version=\"1.0\"><dict><key>Label</key><string>" + plistEscape(label) + "</string><key>ProgramArguments</key><array>" + values.String() + "</array><key>StandardOutPath</key><string>" + plistEscape(logPath) + "</string><key>StandardErrorPath</key><string>" + plistEscape(logPath) + "</string>" + schedule.String() + "</dict></plist>\n"
}

func ensureJobLoaded(domain, label, plistPath string) error {
	if Command("launchctl", "print", domain+"/"+label).Run() == nil {
		return nil
	}
	if err := Command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not load the certificate agent")
	}
	return nil
}

func reloadJob(domain, label, plistPath string) error {
	target := domain + "/" + label
	if Command("launchctl", "print", target).Run() == nil {
		if err := Command("launchctl", "bootout", target).Run(); err != nil {
			return fmt.Errorf("could not reload the certificate agent")
		}
	}
	if err := Command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not reload the certificate agent")
	}
	return nil
}

func EnsureStunnelRunning(p state.Paths) error {
	uid, err := CurrentUID()
	if err != nil {
		return err
	}
	return ensureJobLoaded("gui/"+uid, StunnelLabel, StunnelAgentPath(p))
}

func RestartStunnel(p state.Paths) error {
	if err := EnsureStunnelRunning(p); err != nil {
		return err
	}
	uid, err := CurrentUID()
	if err != nil {
		return err
	}
	if err := Command("launchctl", "kickstart", "-k", "gui/"+uid+"/"+StunnelLabel).Run(); err != nil {
		return fmt.Errorf("could not start the local proxy tunnel")
	}
	return nil
}

func StopStunnel(state.Paths) error {
	uid, err := CurrentUID()
	if err != nil {
		return err
	}
	if err := Command("launchctl", "bootout", "gui/"+uid+"/"+StunnelLabel).Run(); err != nil {
		return fmt.Errorf("could not stop the local proxy tunnel")
	}
	return nil
}

func VerifyProxyOwner(state.Paths) error {
	return nil
}

func plistEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(value)
}
