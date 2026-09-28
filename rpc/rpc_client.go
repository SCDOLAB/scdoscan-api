/**
*  @file
*  @copyright defined in go-scdo/LICENSE
 */

package rpc

import (
	"github.com/scdoproject/scan-api/log"
	"time"
)

// ScdoRPC json_rpc client
type ScdoRPC struct {
	url    string
	scheme string
	conn   *Client
}

// NewRPC create new json_rpc client with given url
func NewRPC(url string, options ...func(rpc *ScdoRPC)) *ScdoRPC {
	rpc := &ScdoRPC{
		url:    url,
		scheme: "tcp",
	}
	for _, option := range options {
		option(rpc)
	}
	return rpc
}

//Connect Create tcp connect
func (rpc *ScdoRPC) Connect() error {
	for rpc.conn == nil {
		conn, err := Dial(rpc.scheme, rpc.url)
		if err != nil {
			log.Error(err)
			time.Sleep(2 * time.Second)
			log.Info("try to reconnect rpc")
		}
		rpc.conn = conn
	}
	return nil
}

//Release release current rpc
func (rpc *ScdoRPC) Release() {
	if rpc != nil && rpc.conn != nil {
		rpc.conn.Close()
		rpc.conn = nil
	}
}

func (rpc *ScdoRPC) call(serviceMethod string, args interface{}, reply interface{}) error {
	if rpc != nil && rpc.conn != nil {
		err := rpc.conn.Call(serviceMethod, args, &reply)
		if err != nil {
			log.Error(err.Error())
			if err.Error()=="connection is shut down" {
				rpc.conn = nil
				rpc.Connect()
			}
			return err
		}
	}else if rpc.conn == nil {
		log.Error("rpc conn is nil, try to connect")
		rpc.Connect()
	}
	return nil
}
