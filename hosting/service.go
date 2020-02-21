package hosting

import (
	"errors"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/digitalautonomy/wahay/config"
	"github.com/digitalautonomy/wahay/tor"
)

var collection Servers

func ensureServerCollection() error {
	var err error

	if collection != nil {
		return nil
	}

	collection, err = Create()
	if err != nil {
		return err
	}

	return nil
}

const (
	defaultPort = 64738
	defaultHost = "127.0.0.1"
)

var errInvalidPort = errors.New("invalid port supplied")

// Service is a representation of our custom Mumble server
type Service interface {
	GetID() string
	GetPort() int
	NewConferenceRoom(password string) error
	Close() error
}

type service struct {
	port       int
	onion      tor.Onion
	room       *conferenceRoom
	httpServer *webserver
}

func (s *service) GetID() string {
	return s.onion.GetID()
}

func (s *service) GetPort() int {
	return s.port
}

type conferenceRoom struct {
	server Server
}

func (s *service) NewConferenceRoom(password string) error {
	serv, err := collection.CreateServer(strconv.Itoa(s.port), password)
	if err != nil {
		return err
	}

	err = serv.Start()
	if err != nil {
		return err
	}

	s.room = &conferenceRoom{
		server: serv,
	}

	// Start our certification http server
	s.httpServer.start()

	return nil
}

func (r *conferenceRoom) close() error {
	return r.server.Stop()
}

// NewService creates a new hosting service
func NewService(port string) (Service, error) {
	err := ensureServerCollection()
	if err != nil {
		return nil, err
	}

	var onionPorts []tor.OnionPort

	httpServer, err := ensureCertificationServer(config.RandomPort(), collection.GetDataDir())
	if err != nil {
		return nil, err
	}

	onionPorts = append(onionPorts, tor.OnionPort{
		DestinationHost: httpServer.host,
		DestinationPort: httpServer.port,
		ServicePort:     8181,
	})

	p := defaultPort
	if port != "" {
		p, err = strconv.Atoi(port)
		if err != nil {
			return nil, errInvalidPort
		}
	}

	serverPort := config.GetRandomPort()

	onionPorts = append(onionPorts, tor.OnionPort{
		DestinationHost: defaultHost,
		DestinationPort: serverPort,
		ServicePort:     p,
	})

	onion, err := tor.NewOnionServiceWithMultiplePorts(onionPorts)
	if err != nil {
		return nil, err
	}

	s := &service{
		port:       serverPort,
		onion:      onion,
		httpServer: httpServer,
	}

	return s, nil
}

var (
	// ErrServerNoClosed is an error to return when the server can't be stopped
	ErrServerNoClosed = errors.New("the current server can't be stopped")
	// ErrServerOnionDelete is an error to return when the hidden service can't be deleted
	ErrServerOnionDelete = errors.New("the hidden service can't be deleted")
)

func (s *service) Close() error {
	var err error

	if s.httpServer != nil {
		err = s.httpServer.stop()
		if err != nil {
			log.Errorf("hosting stop http server: Close(): %s", err)
		}
	}

	if s.room != nil {
		err = s.room.close()
		if err != nil {
			log.Errorf("hosting stop server: Close(): %s", err)
			return ErrServerNoClosed
		}
	}

	if s.onion != nil {
		err = s.onion.Delete()
		if err != nil {
			log.Errorf("hosting delete hidden service: Close(): %s", err)
			return ErrServerOnionDelete
		}
	}

	return nil
}