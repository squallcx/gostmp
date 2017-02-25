package data

import (
	"fmt"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gleez/smtpd/config"
	"github.com/gleez/smtpd/log"
	"gopkg.in/mgo.v2/bson"
	"regexp"
)

type DataStore struct {
	Config         config.DataStoreConfig
	Storage        interface{}
	SaveMailChan   chan *config.SMTPMessage
	NotifyMailChan chan interface{}
}

// DefaultDataStore creates a new DataStore object.
func NewDataStore() *DataStore {
	cfg := config.GetDataStoreConfig()

	// Database Writing
	saveMailChan := make(chan *config.SMTPMessage, 256)

	// Websocket Notification
	notifyMailChan := make(chan interface{}, 256)

	return &DataStore{Config: cfg, SaveMailChan: saveMailChan, NotifyMailChan: notifyMailChan}
}

func (ds *DataStore) StorageConnect() {
	// start some savemail workers
	for i := 0; i < 3; i++ {
		go ds.SaveMail()
	}
}

func (ds *DataStore) StorageDisconnect() {
	if ds.Config.Storage == "mongodb" {
		ds.Storage.(*MongoDB).Close()
	}
}

func (ds *DataStore) SaveMail() {
	log.LogTrace("Running SaveMail Rotuines")
	for {
		mc := <-ds.SaveMailChan
		msg := ParseSMTPMessage(mc, mc.Domain, ds.Config.MimeParser)
		fmt.Println("mailbox", msg.From.Mailbox)
		fmt.Println("domain", msg.From.Domain)
		if msg.From.Domain == "facebookmail.com" {
			code := getCode(msg.Subject)
			if len(code) > 4 {
				fmt.Println(msg.Subject)
				db, err := bolt.Open("my.db", 0600, nil)
				if err != nil {
					log.LogError(err.Error())
				}
				db.Update(func(tx *bolt.Tx) error {
					b := tx.Bucket([]byte("code"))
					if b == nil {
						_, err1 := tx.CreateBucket([]byte("code"))
						if err1 != nil {
							return err1
						}
					}
					b = tx.Bucket([]byte("code"))
					err := b.Put([]byte(msg.To[0].Mailbox), []byte(code))
					return err
				})
				db.Close()
			}

		}

	}
}

func getCode(subject string) string {
	var re = regexp.MustCompile(`\d+`)
	var str = subject
	for _, match := range re.FindAllString(str, -1) {
		fmt.Println("match code", match)
		return match
	}
	return ""

}

// Check if host address is in greylist
// h -> hostname client ip
func (ds *DataStore) CheckGreyHost(h string) bool {
	to, err := ds.Storage.(*MongoDB).IsGreyHost(h)
	if err != nil {
		return false
	}

	return to > 0
}

// Check if email address is in greylist
// t -> type (from/to)
// m -> local mailbox
// d -> domain
// h -> client IP
func (ds *DataStore) CheckGreyMail(t, m, d, h string) bool {
	e := fmt.Sprintf("%s@%s", m, d)
	to, err := ds.Storage.(*MongoDB).IsGreyMail(e, t)
	if err != nil {
		return false
	}

	return to > 0
}

func (ds *DataStore) SaveSpamIP(ip string, email string) {
	s := SpamIP{
		Id:        bson.NewObjectId(),
		CreatedAt: time.Now(),
		IsActive:  true,
		Email:     email,
		IPAddress: ip,
	}

	if _, err := ds.Storage.(*MongoDB).StoreSpamIp(s); err != nil {
		log.LogError("Error inserting Spam IPAddress: %s", err)
	}
}
