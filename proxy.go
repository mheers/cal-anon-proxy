package main

type CalProxy struct {
	config *Config
}

func NewCalProxy(config *Config) *CalProxy {
	return &CalProxy{config: config}
}
