package promotions

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

type Offer struct {
	EventID            string
	ProductID          string
	ProductName        string
	Category           string
	DiscountPercentage float64
	OriginalPrice      float64
	PromotionalPrice   float64
}

type OfferStream struct {
	mu          sync.Mutex
	subscribers map[chan Offer]struct{}
}

func NewOfferStream() *OfferStream {
	return &OfferStream{subscribers: make(map[chan Offer]struct{})}
}

func (stream *OfferStream) Subscribe() (<-chan Offer, func()) {
	offers := make(chan Offer, 16)

	stream.mu.Lock()
	stream.subscribers[offers] = struct{}{}
	stream.mu.Unlock()

	return offers, func() {
		stream.mu.Lock()
		defer stream.mu.Unlock()
		delete(stream.subscribers, offers)
	}
}

func (stream *OfferStream) publish(offer Offer) {
	stream.mu.Lock()
	defer stream.mu.Unlock()

	for subscriber := range stream.subscribers {
		select {
		case subscriber <- offer:
		default:
			log.Printf("[AVISO] Oferta %s descartada para uma assinatura lenta", offer.EventID)
		}
	}
}

func (stream *OfferStream) Run(
	ctx context.Context,
	rabbitURL string,
	publicKeys cryptography.PublicKeyRegistry,
) {
	for ctx.Err() == nil {
		err := stream.consume(ctx, rabbitURL, publicKeys)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[AVISO] Consumo de ofertas interrompido: %v", err)
		}

		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (stream *OfferStream) consume(
	ctx context.Context,
	rabbitURL string,
	publicKeys cryptography.PublicKeyRegistry,
) error {
	connection, err := rabbitmq.ConnectURL(rabbitURL)
	if err != nil {
		return err
	}
	defer connection.Close()

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := rabbitmq.DeclareExchange(
		channel, events.ExchangePromotions, rabbitmq.ExchangeTypeTopic,
	); err != nil {
		return err
	}

	// O RabbitMQ gera o nome. A fila pertence à conexão deste Gateway.
	queue, err := channel.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return err
	}
	if err := rabbitmq.BindQueue(
		channel, queue.Name, events.AllPromotions, events.ExchangePromotions,
	); err != nil {
		return err
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return err
	}

	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	log.Print("[INICIADO] Gateway consumindo ofertas para GraphQL")

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, open := <-deliveries:
			if !open {
				return errors.New("canal de ofertas encerrado")
			}

			var envelope events.EventEnvelope
			if len(delivery.Body) > 64*1024 || json.Unmarshal(delivery.Body, &envelope) != nil {
				log.Print("[REJEITADO] Oferta com tamanho ou JSON inválido")
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}

			payload, err := mspromocoes.ValidatePromotionEvent(
				envelope, delivery.RoutingKey, publicKeys,
			)
			if err != nil {
				log.Printf("[REJEITADO] Oferta: %v", err)
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}

			stream.publish(Offer{
				EventID:            envelope.EventID,
				ProductID:          payload.ProductID,
				ProductName:        payload.ProductName,
				Category:           strings.TrimPrefix(delivery.RoutingKey, "promocao.categoria."),
				DiscountPercentage: payload.DiscountPercentage,
				OriginalPrice:      payload.OriginalPrice,
				PromotionalPrice:   payload.PromotionalPrice,
			})

			if err := delivery.Ack(false); err != nil {
				return err
			}
		}
	}
}
