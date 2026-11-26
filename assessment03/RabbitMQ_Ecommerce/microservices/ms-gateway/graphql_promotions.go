package msgateway

import (
	"context"
	"errors"
	"log"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/promotions"
)

type promotionInterestsResult struct {
	Categories []string
	Email      string
}

type promotionInterestChange struct {
	Changed bool
}

func (resolver *graphQLResolver) InteressesPromocao(
	ctx context.Context,
) (promotionInterestsResult, error) {
	userID, err := resolver.authenticatedUserID(ctx)
	if err != nil {
		return promotionInterestsResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	categories, err := resolver.promotionRepository.ListActive(ctx, userID)
	if err != nil {
		log.Printf("[ERRO] Consulta de interesses: %v", err)
		return promotionInterestsResult{}, errors.New("não foi possível consultar as preferências")
	}
	if categories == nil {
		categories = []string{}
	}

	email, err := resolver.promotionRepository.PreferredEmail(ctx, userID)
	if err != nil {
		log.Printf("[ERRO] Consulta do e-mail de promoções: %v", err)
		return promotionInterestsResult{}, errors.New("não foi possível consultar as preferências")
	}

	return promotionInterestsResult{Categories: categories, Email: email}, nil
}

func (resolver *graphQLResolver) RegistrarInteressePromocao(
	ctx context.Context,
	args *struct {
		Categories []string
		Email      *string
	},
) (promotionInterestChange, error) {
	userID, err := resolver.authenticatedUserID(ctx)
	if err != nil {
		return promotionInterestChange{}, err
	}
	if err := promotions.ValidateCategories(args.Categories); err != nil {
		return promotionInterestChange{}, err
	}

	email := ""
	if args.Email != nil {
		email = *args.Email
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	changed, err := resolver.promotionRepository.SubscribeWithEmail(ctx, userID, args.Categories, email)
	if errors.Is(err, promotions.ErrInvalidEmail) {
		return promotionInterestChange{}, promotions.ErrInvalidEmail
	}
	if err != nil {
		log.Printf("[ERRO] Registro de interesse: %v", err)
		return promotionInterestChange{}, errors.New("não foi possível salvar as preferências")
	}

	return promotionInterestChange{Changed: changed}, nil
}

func (resolver *graphQLResolver) CancelarInteressePromocao(
	ctx context.Context,
	args *struct{ Category string },
) (promotionInterestChange, error) {
	userID, err := resolver.authenticatedUserID(ctx)
	if err != nil {
		return promotionInterestChange{}, err
	}
	if err := promotions.ValidateCategories([]string{args.Category}); err != nil {
		return promotionInterestChange{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	changed, err := resolver.promotionRepository.Unsubscribe(ctx, userID, args.Category)
	if err != nil {
		log.Printf("[ERRO] Cancelamento de interesse: %v", err)
		return promotionInterestChange{}, errors.New("não foi possível cancelar a preferência")
	}

	return promotionInterestChange{Changed: changed}, nil
}
