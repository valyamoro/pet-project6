package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ConnectionParams struct {
	Username string 
	Password string 
	Host string
	Port int 
	DBName string 
}

func NewPostgresConnection(
	ctx context.Context,
	params ConnectionParams,
) (*pgx.Conn, error) {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s",
		params.Username,
		params.Password,
		params.Host,
		params.Port,
		params.DBName,
	)

	return pgx.Connect(ctx, connString)
}
