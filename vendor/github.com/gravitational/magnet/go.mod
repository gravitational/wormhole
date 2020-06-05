module github.com/gravitational/magnet

go 1.14

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.3.1-0.20200512144102-f13ba8f2f2fd
	github.com/docker/docker => github.com/docker/docker v17.12.0-ce-rc1.0.20200310163718-4634ce647cf2+incompatible
	github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
)

require (
	github.com/containerd/console v0.0.0-20191219165238-8375c3424e4d
	github.com/gravitational/trace v1.1.11
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/moby/buildkit v0.7.1
	github.com/opencontainers/go-digest v1.0.0
	github.com/sirupsen/logrus v1.4.2
)
