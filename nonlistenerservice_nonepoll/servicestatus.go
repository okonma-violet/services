package nonlistenerservice_nonepoll

import "sync"

type serviceStatus struct {
	allowedByConf bool
	pubsOK        bool
	sync.RWMutex

	onSuspend   func(string)
	onUnSuspend func()
}

func newServiceStatus() *serviceStatus {
	return &serviceStatus{}
}

func (s *serviceStatus) onAir() bool {
	s.RLock()
	defer s.RUnlock()
	return s.allowedByConf && s.pubsOK
}

func (s *serviceStatus) setPubsStatus(ok bool) {
	s.Lock()
	defer s.Unlock()
	if s.pubsOK == ok {
		return
	}
	if s.allowedByConf {
		if s.pubsOK {
			if s.onSuspend != nil {
				s.onSuspend("publishers not ok")
			}
		} else {
			if s.onUnSuspend != nil {
				s.onUnSuspend()
			}
		}

	}
	s.pubsOK = ok
}
func (s *serviceStatus) setAllowedByConf(ok bool) {
	s.Lock()
	defer s.Unlock()
	if s.allowedByConf == ok {
		return
	}
	if s.pubsOK {
		if s.allowedByConf {
			if s.onSuspend != nil {
				s.onSuspend("listener not ok")
			}
		} else {
			if s.onUnSuspend != nil {
				s.onUnSuspend()
			}
		}

	}
	s.allowedByConf = ok
}

func (s *serviceStatus) setOnSuspendFunc(function func(reason string)) {
	s.Lock()
	defer s.Unlock()

	if s.onSuspend == nil {
		s.onSuspend = function
	} else {
		panic("onSuspend func already set")
	}
}

func (s *serviceStatus) setOnUnSuspendFunc(function func()) {
	s.Lock()
	defer s.Unlock()

	if s.onUnSuspend == nil {
		s.onUnSuspend = function
	} else {
		panic("onUnSuspend func already set")
	}
}

func (s *serviceStatus) isAllowedByConf() bool {
	s.RLock()
	defer s.RUnlock()
	return s.allowedByConf
}

func (s *serviceStatus) isPubsOK() bool {
	s.RLock()
	defer s.RUnlock()
	return s.pubsOK
}
