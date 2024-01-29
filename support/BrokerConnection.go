package support

import "fmt"

type SubSyncOpts struct {
	Timeout int
}

type BrokerConnectionInterface interface {
	Pub(topic string, msg string)
	Sub(uuidItem string, group string, callback func(message string)) (func(), error)
	SubSync(uuidItem string, group string, callback func(message string, err error), opts SubSyncOpts) error
	GetBroker_P() any
	SetKey_P(key string)
	GetKey_P() string
}

func BrokerConnectionSupportContruct() *BrokerConnectionSupport {
	ii := &BrokerConnectionSupport{
		conn_arr: make([]BrokerConnectionInterface, 0),
	}
	return ii
}

type BrokerConnectionSupport struct {
	conn_arr []BrokerConnectionInterface
}

func (c *BrokerConnectionSupport) RegisterConnection(key string, conn BrokerConnectionInterface) {
	conn.SetKey_P(key)
	c.conn_arr = append(c.conn_arr, conn)
}

func (c *BrokerConnectionSupport) GetConnection(key string) BrokerConnectionInterface {
	for _, v := range c.conn_arr {
		if v.GetKey_P() == key {
			fmt.Println("GetConnection :: ", v.GetKey_P(), " == ", key)
			return v.GetBroker_P().(BrokerConnectionInterface)
		}
	}
	return nil
}

func (c *BrokerConnectionSupport) GetObject() any {
	return c
}
