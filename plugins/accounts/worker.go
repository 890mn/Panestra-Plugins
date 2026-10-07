package accounts

import (
	"context"
	"panestra.local/plugins/sdk"
)

func Run(id string) {
	sdk.Run(PluginID(id), func(ctx context.Context, dir string, grants []string, publish sdk.Publish) (sdk.Handler, error) {
		service := NewService(ctx, id, dir, func(status Status, _ uint64) { publish("account.usage", status) })
		return func(ctx context.Context, request sdk.Request) sdk.Response {
			switch request.Operation {
			case "status":
				return sdk.Result(service.Snapshot())
			case "configure":
				if !sdk.CanConfigure(request) {
					return sdk.Denied()
				}
				change, err := sdk.Decode[Change](request.Body)
				if err != nil {
					return sdk.Error(err)
				}
				status, err := service.Configure(change)
				if err != nil {
					return sdk.Error(err)
				}
				return sdk.Result(status)
			case "refresh":
				service.Refresh()
				return sdk.Result(service.Snapshot())
			default:
				return sdk.Failure(404, "NOT_FOUND", "未知插件操作")
			}
		}, nil
	})
}
