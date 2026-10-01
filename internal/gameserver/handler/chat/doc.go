// Package chat holds the rules of a player's chat line before it is
// delivered: the channels the client numbers, which lines are admitted, and
// how an admitted line is cleaned and logged. Delivering a line to its
// listeners is the network layer's job, one handler per channel.
package chat
