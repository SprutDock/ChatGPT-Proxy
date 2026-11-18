package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func installLaunchAgents(p Paths, interactive bool) error {
	if err := ensurePrivateDir(filepath.Dir(p.StunnelAgent)); err != nil {
		return err
	}
	stunnel := filepath.Join(p.Resources, "stunnel")
	if _, err := os.Stat(stunnel); err != nil {
		return fmt.Errorf("bundled stunnel is unavailable")
	}
	uid, err := currentUID()
	if err != nil {
		return err
	}
	domain := "gui/" + uid
	stunnelPlist := launchAgentPlist(stunnelLabel, []string{stunnel, p.StunnelConf}, p.StunnelLog, true, 0)
	if err := writeAtomic(p.StunnelAgent, []byte(stunnelPlist), 0o600); err != nil {
		return err
	}
	agent := filepath.Join(p.Resources, "cert-agent")
	schedulerPlist := launchAgentPlist(agentLabel, []string{agent, "ensure", "--background"}, p.Log, false, 12*60*60)
	if !interactive && available(p.SchedulerAgent) {
		return ensureJobLoaded(domain, agentLabel, p.SchedulerAgent)
	}
	changed, err := writeIfChanged(p.SchedulerAgent, []byte(schedulerPlist), 0o600)
	if err != nil {
		return err
	}
	if changed {
		return reloadJob(domain, agentLabel, p.SchedulerAgent)
	}
	return ensureJobLoaded(domain, agentLabel, p.SchedulerAgent)
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
	if command("launchctl", "print", domain+"/"+label).Run() == nil {
		return nil
	}
	if err := command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not load the certificate agent")
	}
	return nil
}

func reloadJob(domain, label, plistPath string) error {
	target := domain + "/" + label
	if command("launchctl", "print", target).Run() == nil {
		if err := command("launchctl", "bootout", target).Run(); err != nil {
			return fmt.Errorf("could not reload the certificate agent")
		}
	}
	if err := command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not reload the certificate agent")
	}
	return nil
}

func ensureStunnelRunning(p Paths) error {
	uid, err := currentUID()
	if err != nil {
		return err
	}
	return ensureJobLoaded("gui/"+uid, stunnelLabel, p.StunnelAgent)
}

func restartStunnel(p Paths) error {
	if err := ensureStunnelRunning(p); err != nil {
		return err
	}
	uid, err := currentUID()
	if err != nil {
		return err
	}
	if err := command("launchctl", "kickstart", "-k", "gui/"+uid+"/"+stunnelLabel).Run(); err != nil {
		return fmt.Errorf("could not start the local proxy tunnel")
	}
	return nil
}

func stopStunnel(p Paths) error {
	uid, err := currentUID()
	if err != nil {
		return err
	}
	if err := command("launchctl", "bootout", "gui/"+uid+"/"+stunnelLabel).Run(); err != nil {
		return fmt.Errorf("could not stop the local proxy tunnel")
	}
	return nil
}

func plistEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(value)
}
