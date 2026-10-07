package codex

import (
	"context"
	"errors"
	"panestra.local/plugins/sdk"
)

func Run() {
	sdk.Run(ID, func(ctx context.Context, dir string, grants []string, publish sdk.Publish) (sdk.Handler, error) {
		service := NewService(ctx, func(status Status, _ uint64) { publish("account.usage", status) })
		for _, grant := range grants {
			if grant == Permission {
				service.SetEnabled(true)
			}
		}
		return func(ctx context.Context, request sdk.Request) sdk.Response {
			switch request.Operation {
			case "status":
				return sdk.Result(service.Snapshot())
			case "configure":
				if !sdk.CanConfigure(request) {
					return sdk.Denied()
				}
				change, err := sdk.Decode[struct {
					Enabled *bool `json:"enabled"`
				}](request.Body)
				if err != nil {
					return sdk.Error(err)
				}
				if change.Enabled == nil {
					return sdk.Error(errors.New("需要 enabled 字段"))
				}
				service.SetEnabled(*change.Enabled)
				return sdk.Result(service.Snapshot())
			case "refresh":
				service.Refresh()
				return sdk.Result(service.Snapshot())
			default:
				return sdk.Failure(404, "NOT_FOUND", "未知插件操作")
			}
		}, nil
	})
}
