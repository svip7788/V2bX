package limiter

import (
	"regexp"

	"github.com/InazumaV/V2bX/api/panel"
)

func (l *Limiter) CheckDomainRule(destination string) (reject bool) {
	l.mu.RLock()
	rules := l.DomainRules
	l.mu.RUnlock()
	for i := range rules {
		if rules[i].MatchString(destination) {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) CheckProtocolRule(protocol string) (reject bool) {
	l.mu.RLock()
	rules := l.ProtocolRules
	l.mu.RUnlock()
	for i := range rules {
		if rules[i] == protocol {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) UpdateRule(rule *panel.Rules) error {
	rules := make([]*regexp.Regexp, 0, len(rule.Regexp))
	for i := range rule.Regexp {
		r, err := regexp.Compile(rule.Regexp[i])
		if err != nil {
			continue
		}
		rules = append(rules, r)
	}
	l.mu.Lock()
	l.DomainRules = rules
	l.ProtocolRules = rule.Protocol
	l.mu.Unlock()
	return nil
}
