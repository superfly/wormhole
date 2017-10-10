// +build windows

package wormhole

func shellArgs(program string) []string {
	return []string{"cmd.exe", "/C", program}
}
