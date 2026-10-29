package server

import (
	"context"
	"errors"
	"log"

	"connectrpc.com/connect"

	balloonsv1 "github.com/GEHACK/balloons/gen/balloons/v1"
	"github.com/GEHACK/balloons/gen/balloons/v1/balloonsv1connect"
	"github.com/GEHACK/balloons/internal/domjudge"
	"github.com/GEHACK/balloons/internal/state"
)

type Server struct {
	balloonsv1connect.UnimplementedBalloonServiceHandler
	Hub   *Hub
	DJ    *domjudge.Client
	Store *state.Store
}

func (s *Server) ListBalloons(_ context.Context, _ *connect.Request[balloonsv1.ListBalloonsRequest]) (*connect.Response[balloonsv1.ListBalloonsResponse], error) {
	return connect.NewResponse(&balloonsv1.ListBalloonsResponse{
		Balloons:  s.Hub.Snapshot(),
		PrintJobs: s.Hub.PrintJobSnapshot(),
	}), nil
}

func (s *Server) MarkDone(ctx context.Context, req *connect.Request[balloonsv1.MarkDoneRequest]) (*connect.Response[balloonsv1.MarkDoneResponse], error) {
	if err := s.DJ.MarkDone(ctx, req.Msg.BalloonId); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	if err := s.Store.RecordDelivered(req.Msg.BalloonId); err != nil {
		// Don't fail the RPC — DOMjudge is already updated; just log.
		log.Printf("markDone: record delivered: %v", err)
	}
	s.Hub.TriggerRefresh()
	return connect.NewResponse(&balloonsv1.MarkDoneResponse{}), nil
}

func (s *Server) Reprint(_ context.Context, req *connect.Request[balloonsv1.ReprintRequest]) (*connect.Response[balloonsv1.ReprintResponse], error) {
	if err := s.Hub.Reprint(req.Msg.BalloonId); err != nil {
		if errors.Is(err, ErrBalloonNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&balloonsv1.ReprintResponse{}), nil
}

func (s *Server) MarkPrintJobDone(_ context.Context, req *connect.Request[balloonsv1.MarkPrintJobDoneRequest]) (*connect.Response[balloonsv1.MarkPrintJobDoneResponse], error) {
	if err := s.Hub.MarkPrintJobDone(req.Msg.PrintJobId); err != nil {
		if errors.Is(err, ErrPrintJobNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&balloonsv1.MarkPrintJobDoneResponse{}), nil
}

func (s *Server) StreamBalloons(ctx context.Context, _ *connect.Request[balloonsv1.StreamBalloonsRequest], stream *connect.ServerStream[balloonsv1.StreamBalloonsResponse]) error {
	snap, jobs, frozen, ch, unsub := s.Hub.Subscribe()
	defer unsub()

	if err := stream.Send(&balloonsv1.StreamBalloonsResponse{
		Kind:   balloonsv1.StreamBalloonsResponse_KIND_FREEZE,
		Frozen: frozen,
	}); err != nil {
		return err
	}

	for _, b := range snap {
		if err := stream.Send(&balloonsv1.StreamBalloonsResponse{
			Kind:    balloonsv1.StreamBalloonsResponse_KIND_ADDED,
			Balloon: b,
		}); err != nil {
			return err
		}
	}

	for _, j := range jobs {
		if err := stream.Send(&balloonsv1.StreamBalloonsResponse{
			Kind:     balloonsv1.StreamBalloonsResponse_KIND_ADDED,
			PrintJob: j,
		}); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}
