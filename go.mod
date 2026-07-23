module thesinding/teamsctl

go 1.26

toolchain go1.26.5

require (
	github.com/chromedp/cdproto v0.0.0-20241003230502-a4a8f7c660df
	github.com/chromedp/chromedp v0.10.1
	github.com/fossteams/teams-api v0.0.0-20220604181459-dbbdc3681f32
	github.com/zalando/go-keyring v0.2.8
)

replace github.com/fossteams/teams-api => ./third_party/teams-api

require (
	github.com/chromedp/sysutil v1.0.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	golang.org/x/sys v0.27.0 // indirect
)
