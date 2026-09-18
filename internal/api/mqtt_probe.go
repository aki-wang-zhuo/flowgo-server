package api

import (
	"net/http"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/mqttendpoint"
)

// HandleDebugMqttProbe POST /api/flows/{id}/debug/mqtt-probe
// 用当前属性短暂连接 MQTT：收=订阅后断开，发=发布后断开。
func (s *Server) HandleDebugMqttProbe(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req mqttendpoint.ProbeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	out := mqttendpoint.Probe(req, locale)
	status := http.StatusOK
	if !out.OK {
		status = http.StatusOK // 业务失败仍 200，前端靠 ok 字段
	}
	writeJSON(w, status, out)
}
