package glance

import (
	"testing"
	"gopkg.in/yaml.v3"
)

func TestImportYAML(t *testing.T) {
	configYAML := `server:
  host: 127.0.0.1
  port: 8086
  assets-path: ""
branding:
  app-name: Taris Dashboard
  custom-footer: |
    <p>Taris Dashboard</p>
theme:
  light: false
  contrast-multiplier: 1.3
  text-saturation-multiplier: 0.1
pages:
  - name: Home
    head-widgets:
      - cache: 6h
        collapse-after: 2
        feeds:
          - https://selfh.st/rss/
          - https://ciechanow.ski/atom.xml
          - https://www.joshwcomeau.com/rss.xml
          - https://samwho.dev/rss.xml
          - https://ishadeed.com/feed.xml
          - https://feedback.remnote.com/rss/changelog.xml
          - https://rss.sueddeutsche.de/rss/Eilmeldungen
        hide-title: true
        limit: 10
        style: horizontal-cards
        thumbnail-height: 6
        type: rss
    hide-desktop-navigation: true
    columns:
      - size: full
        widgets: [{type: group, widgets: [{type: releases, repositories: [glanceapp/glance, go-gitea/gitea]}, {type: videos, channels: [UCsBjURrPoezykLs9EqgamOA, UCBJycsmduvYEL83R_U4JriQ, UCHnyfMqiRRG1u-2MsSQLbXA, UCKGMHVipEvuZudhHD05FOYA, UCA3mpqm67CpJ13YfA8qAnow]}, {type: markets, style: horizontal-cards, markets: [{symbol: BTC-USD, name: Bitcoin}, {symbol: NVDA, name: NVIDIA}, {symbol: AAPL, name: Apple}, {symbol: MSFT, name: Microsoft}, {symbol: GOOGL, name: Alphabet}, {symbol: GC=F, name: Gold}]}]}]
      - size: small
        widgets: [{hide-title: true, location: Puchheim, type: weather, units: metric}, {hide-title: true, type: spotify}, {hide-title: true, sites: [{title: Orduna, url: 'https://meinorduna.de/de/'}], type: monitor}, {type: neuralwatt, api-key: sk-b3500c15001c50029243bba8d30907d5d329b51a87fd04a850df06daf5fd881c, update-interval: 10}]
spotify:
  client-id: cd11312e353b4adba2e4ab65319cff5b
  client-secret: '40c77c6279fa41a7ac1f864c5671cc2c'
  redirect-url: https://vmd191620.taild84ff9.ts.net/api/spotify/callback
`
	var testConfig Config
	if err := yaml.Unmarshal([]byte(configYAML), &testConfig); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}
	t.Log("Success")
}
