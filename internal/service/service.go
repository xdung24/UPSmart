package service

import "context"

const Name = "UPSmartMonitor"

type Application func(context.Context, func()) error
