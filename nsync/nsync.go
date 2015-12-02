package nsync

type Nsync struct {
	stopCh chan struct{}
}

func NewNsync(stopCh chan struct{}) *Nsync {
	ns := &Nsync{
		stopCh: stopCh,
	}

	return ns
}

func (ns *Nsync) StartServer() {
	NewServer(ns.stopCh).Start()
}
