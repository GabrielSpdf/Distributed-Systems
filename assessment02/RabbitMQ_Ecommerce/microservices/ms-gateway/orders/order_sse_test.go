package orders

import "testing"

func TestOrderSSEHubIsolatesUsers(t *testing.T) {
	hub := newOrderSSEHub()

	first, unsubscribeFirst := hub.subscribe("cliente-1")
	defer unsubscribeFirst()

	second, unsubscribeSecond := hub.subscribe("cliente-2")
	defer unsubscribeSecond()

	event := orderSSEEvent{
		Name: "order.status.changed",
		ID:   "evento-1",
	}

	hub.publish("cliente-1", event)

	select {
	case received := <-first:
		if received.ID != event.ID {
			t.Fatalf("evento incorreto: %q", received.ID)
		}
	default:
		t.Fatal("cliente destinatário não recebeu o evento")
	}

	select {
	case <-second:
		t.Fatal("evento enviado para outro cliente")
	default:
	}
}

func TestOrderSSEHubSupportsMultipleConnections(t *testing.T) {
	hub := newOrderSSEHub()

	first, unsubscribeFirst := hub.subscribe("cliente-1")
	defer unsubscribeFirst()

	second, unsubscribeSecond := hub.subscribe("cliente-1")
	defer unsubscribeSecond()

	hub.publish("cliente-1", orderSSEEvent{ID: "evento-1"})

	for _, channel := range []<-chan orderSSEEvent{first, second} {
		select {
		case received := <-channel:
			if received.ID != "evento-1" {
				t.Fatalf("evento incorreto: %q", received.ID)
			}
		default:
			t.Fatal("uma conexão não recebeu o evento")
		}
	}
}

func TestOrderSSEHubUnsubscribeIsIdempotent(t *testing.T) {
	hub := newOrderSSEHub()

	channel, unsubscribe := hub.subscribe("cliente-1")

	unsubscribe()
	unsubscribe()

	hub.publish("cliente-1", orderSSEEvent{ID: "evento-1"})

	select {
	case _, open := <-channel:
		if open {
			t.Fatal("canal deveria estar fechado")
		}
	default:
		t.Fatal("canal não foi fechado")
	}

	if len(hub.subscribers) != 0 {
		t.Fatal("assinante permaneceu registrado")
	}
}

func TestOrderSSEHubDisconnectsSlowConnection(t *testing.T) {
	hub := newOrderSSEHub()

	slow, unsubscribeSlow := hub.subscribe("cliente-1")
	defer unsubscribeSlow()

	active, unsubscribeActive := hub.subscribe("cliente-1")
	defer unsubscribeActive()

	// Enche a fila lenta, mas mantém a outra conexão consumindo.
	for index := 0; index < cap(slow); index++ {
		hub.publish("cliente-1", orderSSEEvent{ID: "evento"})

		select {
		case <-active:
		default:
			t.Fatal("conexão ativa não recebeu o evento")
		}
	}

	hub.publish("cliente-1", orderSSEEvent{ID: "evento-final"})

	select {
	case received := <-active:
		if received.ID != "evento-final" {
			t.Fatal("conexão ativa recebeu evento incorreto")
		}
	default:
		t.Fatal("conexão lenta prejudicou a conexão ativa")
	}

	// Um canal fechado ainda permite ler os eventos já armazenados.
	for index := 0; index < cap(slow); index++ {
		select {
		case _, open := <-slow:
			if !open {
				t.Fatal("fila fechou antes de drenar os eventos")
			}
		default:
			t.Fatal("evento armazenado não encontrado")
		}
	}

	select {
	case _, open := <-slow:
		if open {
			t.Fatal("conexão lenta deveria estar fechada")
		}
	default:
		t.Fatal("conexão lenta não foi encerrada")
	}

	if len(hub.subscribers["cliente-1"]) != 1 {
		t.Fatal("deveria restar apenas a conexão ativa")
	}
}
