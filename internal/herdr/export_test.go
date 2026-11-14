package herdr

// EarlyFrames counts the frames the client holds for channels that have no
// stream: ones that came before their attach reply was read, or after their
// stream closed.
func EarlyFrames(c *Client) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, frames := range c.early {
		n += len(frames)
	}
	return n
}

// GoneChannels counts the channels the client closed and still waits on for
// the core's answering CLOSE.
func GoneChannels(c *Client) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.gone)
}
