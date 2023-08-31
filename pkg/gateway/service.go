package gateway

import (
	"GradingCore2/pkg/grading"
	"context"
	"encoding/json"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"sync"
	"time"
)

const (
	ExchangeName       = "grading"
	RequestQueueName   = "grading_request"
	ResponseQueueName  = "grading_response"
	RoutingKeyRequest  = "request"
	RoutingKeyResponse = "response"
)

type Service struct {
	AmqpUrl        string
	AmqpConnection *amqp.Connection
	AmqpChannel    *amqp.Channel
	AmqpQueue      amqp.Queue
	Running        bool

	Concurrency    int
	RunningCount   int
	BackoffCounter int
	Lock           sync.Mutex
	GradingService *grading.Service
}

const BackoffAmount = 4

func NewService(amqpUrl string, concurrency int, gradingService *grading.Service) Service {
	return Service{
		AmqpUrl:        amqpUrl,
		Running:        true,
		Concurrency:    concurrency,
		RunningCount:   0,
		Lock:           sync.Mutex{},
		GradingService: gradingService,
	}
}

func (s *Service) ConnectAmqp() error {
	dial, err := amqp.Dial(s.AmqpUrl)
	if err != nil {
		return err
	}
	s.AmqpConnection = dial

	channel, err := dial.Channel()
	if err != nil {
		return err
	}
	s.AmqpChannel = channel

	_, err = channel.QueueDeclare(RequestQueueName, false, false, false, false, nil)
	if err != nil {
		return err
	}

	_, err = channel.QueueDeclare(ResponseQueueName, false, false, false, false, nil)
	if err != nil {
		return err
	}

	err = channel.ExchangeDeclare(ExchangeName, amqp.ExchangeTopic, false, false, false, false, nil)
	if err != nil {
		return err
	}

	err = channel.QueueBind(RequestQueueName, RoutingKeyRequest, ExchangeName, false, nil)
	if err != nil {
		return err
	}

	err = channel.QueueBind(ResponseQueueName, RoutingKeyResponse, ExchangeName, false, nil)
	if err != nil {
		return err
	}

	log.Println("AMQP connected", s.AmqpUrl)
	return nil
}

func (s *Service) Tick() error {
	if s.AmqpConnection == nil || s.AmqpConnection.IsClosed() {
		err := s.ConnectAmqp()
		if err != nil {
			return err
		}
	}

	if s.BackoffCounter > 0 {
		s.BackoffCounter--
		return nil
	}

	s.Lock.Lock()
	defer s.Lock.Unlock()
	if s.RunningCount < s.Concurrency {
		msg, ok, err := s.AmqpChannel.Get(RequestQueueName, false)
		if err != nil {
			s.BackoffCounter = BackoffAmount
			return err
		}
		if !ok {
			s.BackoffCounter = BackoffAmount
			return nil
		}

		s.RunningCount++
		log.Println("gateway started", s.RunningCount)

		go func() {
			handlerErr := s.HandleDelivery(&msg)
			if handlerErr == nil {
				err = msg.Ack(false)
				if err != nil {
					log.Println("failed to ack", err)
				}
			} else {
				log.Println(handlerErr)
				nackErr := msg.Nack(false, true)
				if nackErr != nil {
					log.Println("failed to nack", nackErr)
				}
			}

			s.Lock.Lock()
			s.RunningCount = s.RunningCount - 1
			log.Println("gateway finished", s.RunningCount)
			s.Lock.Unlock()
		}()
	}

	return nil
}

func (s *Service) HandleDelivery(delivery *amqp.Delivery) error {
	ctx := context.Background()
	var req grading.Request
	err := json.Unmarshal(delivery.Body, &req)
	if err != nil {
		return err
	}

	grade, err := s.GradingService.Grade(ctx, &req)
	if err != nil {
		return err
	}

	marshal, err := json.Marshal(grade)
	if err != nil {
		return err
	}

	log.Println(string(marshal))
	message := amqp.Publishing{
		Body: marshal,
	}
	err = s.AmqpChannel.PublishWithContext(ctx, ExchangeName, RoutingKeyResponse, false, false, message)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) Run() {
	for s.Running {
		time.Sleep(time.Second * 15)
		err := s.Tick()
		if err != nil {
			log.Println(err)
		}
	}
}