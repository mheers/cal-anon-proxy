package main

import (
	"context"
	"log"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	SrcUpdateInterval int `env:"SRC_UPDATE_INTERVAL" default:"5"`

	Src1URL      string `env:"SRC_1_URL"`
	Src1Anon     bool   `env:"SRC_1_ANON"`
	Src1Username string `env:"SRC_1_USERNAME"`
	Src1Password string `env:"SRC_1_PASSWORD"`

	Src2URL      string `env:"SRC_2_URL"`
	Src2Anon     bool   `env:"SRC_2_ANON"`
	Src2Username string `env:"SRC_2_USERNAME"`
	Src2Password string `env:"SRC_2_PASSWORD"`

	Src3URL      string `env:"SRC_3_URL"`
	Src3Anon     bool   `env:"SRC_3_ANON"`
	Src3Username string `env:"SRC_3_USERNAME"`
	Src3Password string `env:"SRC_3_PASSWORD"`

	DstAuthEnabled  bool   `env:"DST_AUTH_ENABLED"`
	DstUsername     string `env:"DST_USERNAME"`
	DstPassword     string `env:"DST_PASSWORD"`
	DstPublicDomain string `env:"DST_PUBLIC_DOMAIN"`
}

func ReadConfig() *Config {
	var c Config
	if err := envconfig.Process(context.Background(), &c); err != nil {
		log.Fatal(err)
	}
	return &c
}

func (c *Config) Srcs() []*Src {
	srcs := []*Src{}
	if c.Src1URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src1URL,
			Anon:     c.Src1Anon,
			Username: c.Src1Username,
			Password: c.Src1Password,
		})
	}

	if c.Src2URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src2URL,
			Anon:     c.Src2Anon,
			Username: c.Src2Username,
			Password: c.Src2Password,
		})
	}

	if c.Src3URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src3URL,
			Anon:     c.Src3Anon,
			Username: c.Src3Username,
			Password: c.Src3Password,
		})
	}

	return srcs
}

type Src struct {
	URL      string
	Anon     bool
	Username string
	Password string
}
