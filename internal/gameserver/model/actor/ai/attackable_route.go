package ai

// Route is the walker route an actor walks while its route desire is the
// one its AI acts on. Its calls run on the actor's queue with the AI mutex
// held, so it never calls back into the AI.
type Route interface {
	// Walk puts the actor on the route named name. An actor not on it heads
	// for the route node nearest to it; one already on it walks on.
	Walk(name string)
	// Leave takes the actor off its route while its AI acts on another
	// desire: neither an arrival nor the end of a node's pause moves it on
	// until the next Walk.
	Leave()
}

// SetRoute gives the AI the route its route desires walk. Call it once,
// before the actor is published. Left unset, a route desire is acted on
// but moves the actor nowhere, as for a route with no node.
func (a *Attackable) SetRoute(r Route) {
	a.route = r
}

// AddMoveRouteDesire queues a request to walk the route named name with
// weight; an equal desire already queued gains the weight instead. The
// desire never loses weight and stays queued for the actor's life, so the
// actor walks the route whenever nothing outweighs it.
func (a *Attackable) AddMoveRouteDesire(name string, weight float64) {
	a.desires.AddOrUpdate(&Desire{
		Kind:      IntentionMoveRoute,
		RouteName: name,
		Weight:    weight,
		QueuedAt:  a.now(),
	})
}

// thinkMoveRoute steps the current route intention: an actor not on its
// route heads for the nearest node, one already on it walks on undisturbed.
// Callers hold mu.
func (a *Attackable) thinkMoveRoute() {
	if a.route == nil {
		return
	}
	a.onRoute = true
	a.route.Walk(a.current.route)
}

// leaveRoute takes the actor off its route as its AI takes up another
// desire. Callers hold mu.
func (a *Attackable) leaveRoute() {
	if !a.onRoute {
		return
	}
	a.onRoute = false
	a.route.Leave()
}
