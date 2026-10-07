package netease

import (
	"context"
	"errors"
	"panestra.local/plugins/sdk"
)

func Run() {
	sdk.Run(ID, func(ctx context.Context, dir string, grants []string, publish sdk.Publish) (sdk.Handler, error) {
		service := NewService(ctx, dir, func(status Status, _ uint64) { publish("media.status", status) })
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
			case "control":
				if !sdk.CanControl(request) {
					return sdk.Denied()
				}
				action, err := sdk.Decode[Action](request.Body)
				if err != nil {
					return sdk.Error(err)
				}
				if err = service.Control(ctx, action); err != nil {
					var problem *Error
					if errors.As(err, &problem) && problem.State == "forbidden" {
						return sdk.Denied()
					}
					return sdk.Error(err)
				}
				service.Refresh()
				return sdk.Result(service.Snapshot())
			default:
				return sdk.Failure(404, "NOT_FOUND", "未知插件操作")
			}
		}, nil
	})
}
