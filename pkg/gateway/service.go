package gateway

import (
	"GradingCore2/pkg/grading"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		return fmt.Errorf("AMQP failed to dial server: %w", err)
	}
	s.AmqpConnection = dial

	channel, err := dial.Channel()
	if err != nil {
		return fmt.Errorf("AMQP failed to get channel %w", err)
	}
	s.AmqpChannel = channel

	_, err = channel.QueueDeclare(RequestQueueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("AMQP failed to declare request queue: %w", err)
	}

	_, err = channel.QueueDeclare(ResponseQueueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("AMQP failed to declare response queue: %w", err)
	}

	err = channel.ExchangeDeclare(ExchangeName, amqp.ExchangeTopic, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("AMQP failed to declare exchange: %w", err)
	}

	err = channel.QueueBind(RequestQueueName, RoutingKeyRequest, ExchangeName, false, nil)
	if err != nil {
		return fmt.Errorf("AMQP failed to bind request queue: %w", err)
	}

	err = channel.QueueBind(ResponseQueueName, RoutingKeyResponse, ExchangeName, false, nil)
	if err != nil {
		return fmt.Errorf("AMQP failed to bind response queue: %w", err)
	}

	//err = channel.Qos(4, 0, false)
	//if err != nil {
	//	return err
	//}
	//
	//consume, err := channel.Consume(RequestQueueName, "", false, false, false, false, nil)
	//if err != nil {
	//	return err
	//}

	log.Println("AMQP connected", s.AmqpUrl)
	return nil
}

func (s *Service) Tick() error {
	if s.AmqpConnection == nil || s.AmqpConnection.IsClosed() {
		err := s.ConnectAmqp()
		if err != nil {
			return fmt.Errorf("attempt to connect AMQP failed while ticking: %w", err)
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
			return fmt.Errorf("dequeue failed: %w", err)
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
	decoder := json.NewDecoder(bytes.NewBuffer(delivery.Body))
	decoder.UseNumber()
	err := decoder.Decode(&req)

	if err != nil {
		return err
	}
	log.Println("req", string(delivery.Body))
	grade, gradingError := s.GradingService.Grade(ctx, &req)
	if gradingError != nil {
		log.Println("grading error", gradingError)
		nackError := delivery.Nack(false, true)
		if nackError != nil {
			log.Println("Nack error")
		}
		return err
	}

	err = s.Publish(ctx, grade)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) Publish(ctx context.Context, response *grading.Response) error {
	marshal, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed marshal while publishing message to queue: %w", err)
	}
	log.Println("res", string(marshal))
	message := amqp.Publishing{
		Body: marshal,
	}
	err = s.AmqpChannel.PublishWithContext(ctx, ExchangeName, RoutingKeyResponse, false, false, message)
	if err != nil {
		return fmt.Errorf("failed publishing message to queue: %w", err)
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
