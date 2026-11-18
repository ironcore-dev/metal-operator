package image

func JoinCmdline(cmdline, args string) string {
	if args == "" {
		return cmdline
	}
	return cmdline + " " + args
}
