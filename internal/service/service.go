package service

import (
	"context"

	systemservice "github.com/kardianos/service"
)

const Name = "UPSmart"

type Application func(context.Context, func()) error

func Interactive() bool {
	return systemservice.Interactive()
}
