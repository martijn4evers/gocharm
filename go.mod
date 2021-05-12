module github.com/mever/gocharm

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/juju/charm/v9 v9.0.0-20210512004933-c21e01ffd4ad
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/mgo/v2 v2.0.0-20210414025616-e854c672032f
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/os/v2 v2.1.2 // indirect
	github.com/juju/schema v1.0.0 // indirect
	github.com/juju/testing v0.0.0-20210302031854-2c7ee8570c07
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2 // indirect
	github.com/kardianos/service v1.2.0 // indirect
	github.com/mever/service v1.2.1-0.20210512123113-570438e960f8
	github.com/pkg/errors v0.9.1
	golang.org/x/crypto v0.0.0-20210506145944-38f3c27a63bf // indirect
	golang.org/x/net v0.0.0-20210510120150-4163338589ed // indirect
	golang.org/x/sys v0.0.0-20210511113859-b0526f3d8744 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/kardianos/service v1.2.0 => github.com/mever/service v1.2.0
