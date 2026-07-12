package api

//go:generate go tool oapi-codegen --generate "types,std-http-server,strict-server" --package api -o api.gen.go Ticket_Allocation.swagger.yaml
