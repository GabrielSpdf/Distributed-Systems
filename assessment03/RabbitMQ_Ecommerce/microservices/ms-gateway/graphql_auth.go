package msgateway

import (
	"context"
	"errors"
	"net/http"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
)

type userResult struct {
	ID        string
	Name      string
	Email     string
	CreatedAt string
}

func toUserResult(user auth.User) *userResult {
	return &userResult{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Format(time.RFC3339Nano),
	}
}

func (resolver *graphQLResolver) CadastrarUsuario(
	ctx context.Context,
	args *struct{ Name, Email, Password string },
) (*userResult, error) {
	user, err := resolver.authentication.Register(ctx, args.Name, args.Email, args.Password)
	if err != nil {
		return nil, err
	}
	return toUserResult(user), nil
}

func (resolver *graphQLResolver) Entrar(
	ctx context.Context,
	args *struct{ Email, Password string },
) (*userResult, error) {
	writer, ok := ctx.Value(graphQLWriterKey{}).(http.ResponseWriter)
	if !ok {
		return nil, errors.New("resposta HTTP indisponível")
	}

	user, err := resolver.authentication.Login(ctx, writer, args.Email, args.Password)
	if err != nil {
		return nil, err
	}
	return toUserResult(user), nil
}

func (resolver *graphQLResolver) UsuarioAtual(ctx context.Context) (*userResult, error) {
	request, ok := ctx.Value(graphQLRequestKey{}).(*http.Request)
	if !ok {
		return nil, errors.New("requisição indisponível")
	}

	user, status, message := resolver.authentication.CurrentUser(request.WithContext(ctx))
	if status != http.StatusOK {
		return nil, errors.New(message)
	}
	return toUserResult(user), nil
}

func (resolver *graphQLResolver) Sair(ctx context.Context) (bool, error) {
	writer, ok := ctx.Value(graphQLWriterKey{}).(http.ResponseWriter)
	if !ok {
		return false, errors.New("resposta HTTP indisponível")
	}

	resolver.authentication.Logout(writer)
	return true, nil
}
