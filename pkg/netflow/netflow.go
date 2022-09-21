package netflow

import (
	"context"
	"net/url"
	"strconv"

	"github.com/netsampler/goflow2/format"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type NFReader struct {
	dispatcher Dispatcher
	workers    int
	url        *url.URL
	log        *logrus.Logger
}

// Consumer is the interface that must be implemented to consume the NetFlow data.
type Consumer interface {
	Consume(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger)
}

// Enricher is the interface that must be implemented to enrich the NetFlow data.
type Enricher interface {
	Enrich(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) map[string]interface{}
}

// Implements goflow2.transport.TransportDriver
type Dispatcher struct {
	consumer  Consumer
	enrichers []Enricher
	log       *logrus.Logger
}

func (d *Dispatcher) Prepare() error {
	return nil
}
func (d *Dispatcher) Init(context.Context) error {
	return nil
}
func (d *Dispatcher) Close(context.Context) error {
	return nil
}
func (d *Dispatcher) Send(key, data []byte) error {
	var msg flowmessage.FlowMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		d.log.Errorf("Wrong Flow Message (%s) : %s", err.Error(), string(data))
		return err
	}

	var extra map[string]interface{}
	for _, enricher := range d.enrichers {
		extra = enricher.Enrich(&msg, extra, d.log)
	}
	d.consumer.Consume(&msg, extra, d.log)
	return nil
}

func NewNFReader(workers int, address string, consumer Consumer, enrichers []Enricher, log *logrus.Logger) (*NFReader, error) {
	url, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = logrus.New()
	}

	return &NFReader{
		dispatcher: Dispatcher{
			consumer:  consumer,
			enrichers: enrichers,
			log:       log,
		},
		workers: workers,
		url:     url,
		log:     log,
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
		Transport: &r.dispatcher,
		Logger:    r.log,
	}
	err = sNF.FlowRoutine(r.workers, hostname, int(port), false)

	if err != nil {
		r.log.WithFields(logFields).Fatal(err)
	}
}
