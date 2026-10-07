package alas

import (
	"context"
	"panestra.local/plugins/sdk"
)

func Run() {
	sdk.Run(ID, func(ctx context.Context, dir string, grants []string, publish sdk.Publish) (sdk.Handler, error) {
		service := NewService(ctx, dir, func(status Status, _ uint64) { publish("task.status", status) })
		return func(ctx context.Context, request sdk.Request) sdk.Response {
			switch request.Operation {
			case "status":
				return sdk.Result(service.Snapshot())
			case "setup":
				if !sdk.CanConfigure(request) {
					return sdk.Denied()
				}
				return sdk.Result(service.Setup())
			case "configure", "prepare":
				if !sdk.CanConfigure(request) {
					return sdk.Denied()
				}
				change, err := sdk.Decode[Change](request.Body)
				if err != nil {
					return sdk.Error(err)
				}
				status, err := service.Configure(change, request.Operation == "prepare")
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
