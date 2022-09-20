package netflow

import (
	"context"
	"net/url"
	"strconv"

	"github.com/netsampler/goflow2/format"
	"github.com/netsampler/goflow2/transport"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
)

type NFReader struct {
	transport transport.TransportDriver
	workers   int
	url       *url.URL
	log       *logrus.Logger
}

func NewNFReader(transport transport.TransportDriver, workers int, address string, log *logrus.Logger) (*NFReader, error) {
	url, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = logrus.New()
	}

	return &NFReader{
		transport: transport,
		workers:   workers,
		url:       url,
		log:       log,
	}, nil

}

// Read starts listening to the configured address. Can (and should) be run from
// within a goroutine.
func (r *NFReader) Listen() {
	hostname := r.url.Hostname()
	port, err := strconv.ParseUint(r.url.Port(), 10, 64)
	if err != nil {
		r.log.Errorf("Port %s could not be converted to integer", r.url.Port())
		return
	}
	logFields := logrus.Fields{
		"scheme":   r.url.Scheme,
		"hostname": hostname,
		"port":     port,
	}
	r.log.WithFields(logFields).Info("Starting collection on " + r.url.String())

	formatter, err := format.FindFormat(context.Background(), "pb")
	sNF := &utils.StateNetFlow{
		Format:    formatter,
		Transport: r.transport,
		Logger:    r.log,
	}
	err = sNF.FlowRoutine(r.workers, hostname, int(port), false)

	if err != nil {
		r.log.WithFields(logFields).Fatal(err)
	}
}
