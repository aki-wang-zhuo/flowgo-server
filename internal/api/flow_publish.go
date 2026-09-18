package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo-server/internal/store"
)

type publishReq struct {
	Note string `json:"note"`
}

type rollbackReq struct {
	Version int `json:"version"`
}

func (s *Server) writeFlowStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrFlowLocked) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, store.ErrNotPublished) || errors.Is(err, store.ErrNoPublished) || errors.Is(err, store.ErrHistoryNotFound) || errors.Is(err, store.ErrCannotDeleteLive) || errors.Is(err, store.ErrNoPublishHistory) || errors.Is(err, store.ErrAlreadyOnline) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func (s *Server) afterPublish(rec *store.FlowRecord) error {
	if s.Endpoints != nil {
		if err := s.Endpoints.SyncFlow(rec); err != nil {
			return err
		}
	}
	if s.Exec != nil {
		s.Exec.InvalidateFlowTrack(rec.ID, engine.CacheTrackPublished)
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("published", rec.ID, rec.Name, "api")
	}
	return nil
}

// HandlePublishFlow POST /api/flows/{id}/publish
// 将草稿发布为线上版本并同步 HTTP 入口；未发布流程此前不监听。
func (s *Server) HandlePublishFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req publishReq
	_ = decodeJSON(r, &req)

	rec, err := s.Store.GetFlow(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if rec.Locked {
		writeError(w, http.StatusConflict, "flow is locked")
		return
	}
	if rec.DSL == nil {
		writeError(w, http.StatusBadRequest, "draft dsl is empty")
		return
	}
	if s.Endpoints != nil {
		if err := s.Endpoints.CheckHTTPRoutes(id, rec.DSL); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	rec, err = s.Store.PublishFlow(id, req.Note)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if err := s.afterPublish(rec); err != nil {
		writeError(w, http.StatusBadRequest, "published but http endpoint failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleUnpublishFlow POST /api/flows/{id}/offline
// 下线：撤销当前发布（归档进历史），从内存卸载 HTTP；草稿与调试不受影响。
func (s *Server) HandleUnpublishFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rec, err := s.Store.GetFlow(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if rec.Locked {
		writeError(w, http.StatusConflict, "flow is locked")
		return
	}
	rec, err = s.Store.UnpublishFlow(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if err := s.afterOffline(rec); err != nil {
		writeError(w, http.StatusBadRequest, "unpublished but unload failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleGoOnlineFlow POST /api/flows/{id}/online
// 上线：从历史最近一条已发布快照恢复线上版并挂载 HTTP；无历史则拒绝。
func (s *Server) HandleGoOnlineFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rec, err := s.Store.GetFlow(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if rec.Locked {
		writeError(w, http.StatusConflict, "flow is locked")
		return
	}
	if rec.Published {
		writeError(w, http.StatusConflict, store.ErrAlreadyOnline.Error())
		return
	}
	peek, err := s.Store.PeekLatestPublishDSL(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if s.Endpoints != nil {
		if err := s.Endpoints.CheckHTTPRoutes(id, peek); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	rec, err = s.Store.GoOnlineFromLatest(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if err := s.afterPublish(rec); err != nil {
		writeError(w, http.StatusBadRequest, "online but http endpoint failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) afterOffline(rec *store.FlowRecord) error {
	if s.Endpoints != nil {
		if err := s.Endpoints.SyncFlow(rec); err != nil {
			return err
		}
		if err := s.Endpoints.SyncDraftIfOpen(rec); err != nil {
			// 下线后草稿「连接并响应」失败不阻断下线
			_ = err
		}
	}
	if s.Exec != nil {
		s.Exec.InvalidateFlowTrack(rec.ID, engine.CacheTrackPublished)
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("offline", rec.ID, rec.Name, "api")
	}
	return nil
}

// HandleDiscardDraft POST /api/flows/{id}/discard-draft
func (s *Server) HandleDiscardDraft(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rec, err := s.Store.DiscardDraft(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if s.Exec != nil {
		s.Exec.InvalidateFlowTrack(id, engine.CacheTrackDraft)
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("discard-draft", rec.ID, rec.Name, "api")
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleListPublishHistory GET /api/flows/{id}/publish-history
func (s *Server) HandleListPublishHistory(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	include := r.URL.Query().Get("includeDsl") == "true"
	list, rec, err := s.Store.ListPublishHistory(id, include)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"flowId":             rec.ID,
		"published":          rec.Published,
		"publishedVersion":   rec.PublishedVersion,
		"unpublishedChanges": rec.UnpublishedChanges,
		"items":              list,
	})
}

// HandleRollbackPublish POST /api/flows/{id}/rollback
func (s *Server) HandleRollbackPublish(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req rollbackReq
	if err := decodeJSON(r, &req); err != nil || req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version is required")
		return
	}
	rec, err := s.Store.GetFlow(id)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if rec.Locked {
		writeError(w, http.StatusConflict, "flow is locked")
		return
	}
	hist, _, err := s.Store.ListPublishHistory(id, true)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	var target *store.PublishHistoryItem
	for i := range hist {
		if hist[i].Version == req.Version {
			target = &hist[i]
			break
		}
	}
	if target == nil || target.DSL == nil {
		writeError(w, http.StatusConflict, store.ErrHistoryNotFound.Error())
		return
	}
	if s.Endpoints != nil {
		if err := s.Endpoints.CheckHTTPRoutes(id, target.DSL); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	rec, err = s.Store.RollbackPublish(id, req.Version)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if err := s.afterPublish(rec); err != nil {
		writeError(w, http.StatusBadRequest, "rolled back but http endpoint failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleDeletePublishHistory DELETE /api/flows/{id}/publish-history/{version}
// 删除历史快照；禁止删除当前线上版本。
func (s *Server) HandleDeletePublishHistory(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	verStr := r.PathValue("version")
	if id == "" || verStr == "" {
		writeError(w, http.StatusBadRequest, "missing id or version")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	ver, err := strconv.Atoi(verStr)
	if err != nil || ver <= 0 {
		writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	rec, err := s.Store.DeletePublishHistory(id, ver)
	if err != nil {
		s.writeFlowStoreErr(w, err)
		return
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("history-deleted", rec.ID, rec.Name, "api")
	}
	writeJSON(w, http.StatusOK, rec)
}
