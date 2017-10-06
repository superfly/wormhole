// +build !windows

package wormhole

func shellArgs(program string) []string {
	return []string{"/bin/sh", "-c", program}
}
