package protocol

import (
	rm "encoding/json"
	"sync"

	"github.com/goccy/go-json"
	"github.com/roadrunner-server/api/v4/plugins/v1/jobs"
	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/sdk/v4/payload"
	"go.uber.org/zap"
)

type Type uint32

const (
	NoError Type = iota
	Error
	Response
)

// internal worker protocol (jobs mode)
type protocol struct {
	// message type, see Type
	T Type `json:"type"`
	// Payload
	Data rm.RawMessage `json:"data"`
}

type RespHandler struct {
	log *zap.Logger
	// response pools
	qPool sync.Pool
	ePool sync.Pool
	pPool sync.Pool
}

func NewResponseHandler(log *zap.Logger) *RespHandler {
	return &RespHandler{
		log: log,

		pPool: sync.Pool{
			New: func() any {
				return new(protocol)
			},
		},

		qPool: sync.Pool{
			New: func() any {
				return new(queueResp)
			},
		},

		ePool: sync.Pool{
			New: func() any {
				return new(errorResp)
			},
		},
	}
}

func (rh *RespHandler) Handle(pld *payload.Payload, jb jobs.Acknowledger) error {
	const op = errors.Op("jobs_handle_response")
	p := rh.getProtocol()
	defer rh.putProtocol(p)

	err := json.Unmarshal(pld.Body, p)
	if err != nil {
		return errors.E(op, err)
	}

	switch p.T {
	// likely case
	case NoError:
		err = jb.Ack()
		if err != nil {
			return errors.E(op, err)
		}
		return nil
		// error returned from the PHP
	case Error:
		err = rh.handleErrResp(p.Data, jb)
		if err != nil {
			return errors.E(op, err)
		}
		return nil
		// RR should send a response to the queue/tube/subject
	case Response:
		err = rh.handleQueueResp(p.Data, jb)
		if err != nil {
			return err
		}
		return nil
	default:
		err = jb.Ack()
		if err != nil {
			return errors.E(op, err)
		}
	}

	return nil
}

func (rh *RespHandler) getProtocol() *protocol {
	return rh.pPool.Get().(*protocol)
}

func (rh *RespHandler) putProtocol(p *protocol) {
	p.T = 0
	p.Data = nil
	rh.pPool.Put(p)
}

func (rh *RespHandler) getErrResp() *errorResp {
	return rh.ePool.Get().(*errorResp)
}

func (rh *RespHandler) putErrResp(p *errorResp) {
	p.Msg = ""
	p.Headers = nil
	p.Delay = 0
	p.Requeue = false
	rh.ePool.Put(p)
}

func (rh *RespHandler) getQResp() *queueResp {
	return rh.qPool.Get().(*queueResp)
}

func (rh *RespHandler) putQResp(p *queueResp) {
	p.Queue = ""
	rh.qPool.Put(p)
}
