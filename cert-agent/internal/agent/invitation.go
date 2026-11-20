package agent

func validInvitation(invitation string) bool {
	return len(invitation) >= 32 && len(invitation) <= 256
}
