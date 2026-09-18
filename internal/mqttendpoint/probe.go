/**
 * MQTT 连接探测：用当前属性短暂连接，测完立即断开。
 * - subscribe：连接并订阅，成功/失败都断开
 * - publish：连接并发布一条消息（可用载荷模板原文），然后断开
 */
package mqttendpoint

import (
	"fmt"
	"strings"
	"time"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components/iot"
	"github.com/flowgo/flowgo/utils/mqtt"
)

// ProbeKind 探测类型。
const (
	ProbeSubscribe = "subscribe"
	ProbePublish   = "publish"
)

// ProbeRequest 探测请求（由编辑器传入当前面板配置）。
type ProbeRequest struct {
	Kind string `json:"kind"` // subscribe | publish
	// Configuration 节点 configuration；publish 且 reuseFrom 非空时需带 ReuseConfiguration。
	Configuration map[string]interface{} `json:"configuration"`
	// ReuseConfiguration 复用的 mqttIn configuration（仅 publish + reuseFrom）。
	ReuseConfiguration map[string]interface{} `json:"reuseConfiguration,omitempty"`
}

// ProbeResult 探测结果。
type ProbeResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Probe 执行一次短暂探测并保证断开。
func Probe(req ProbeRequest, locale string) ProbeResult {
	locale = types.NormalizeLocale(locale)
	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case ProbeSubscribe:
		return probeSubscribe(req.Configuration, locale)
	case ProbePublish:
		return probePublish(req, locale)
	default:
		return failResult(locale, "不支持的探测类型", "unsupported probe kind")
	}
}

func probeSubscribe(config map[string]interface{}, locale string) ProbeResult {
	cfg, err := iot.ParseMqttInConfig(config)
	if err != nil {
		return failResult(locale, "配置无效: "+err.Error(), "invalid config: "+err.Error())
	}
	broker := cfg.Broker
	if strings.TrimSpace(broker.ClientID) == "" {
		broker.ClientID = fmt.Sprintf("flowgo-probe-%d", time.Now().UnixNano()%1e9)
	}
	client, err := mqtt.Connect(broker, 10*time.Second)
	if err != nil {
		return failResult(locale, "连接失败: "+err.Error(), "connect failed: "+err.Error())
	}
	defer client.Disconnect(250)

	token := client.Subscribe(cfg.Topic, cfg.QoS, nil)
	if !token.WaitTimeout(8 * time.Second) {
		return failResult(locale, "订阅超时: "+cfg.Topic, "subscribe timeout: "+cfg.Topic)
	}
	if err := token.Error(); err != nil {
		return failResult(locale, "订阅失败: "+err.Error(), "subscribe failed: "+err.Error())
	}
	return okResult(locale,
		fmt.Sprintf("连接并订阅成功（topic=%s qos=%d），已断开", cfg.Topic, cfg.QoS),
		fmt.Sprintf("Connected and subscribed (topic=%s qos=%d), disconnected", cfg.Topic, cfg.QoS),
	)
}

func probePublish(req ProbeRequest, locale string) ProbeResult {
	cfg, err := iot.ParseMqttOutConfig(req.Configuration)
	if err != nil {
		return failResult(locale, "配置无效: "+err.Error(), "invalid config: "+err.Error())
	}
	broker := cfg.Broker
	if cfg.ReuseFrom != "" {
		reuse, err := iot.ParseMqttInConfig(req.ReuseConfiguration)
		if err != nil {
			return failResult(locale,
				"复用 MQTT 收配置无效: "+err.Error(),
				"reuse MQTT In config invalid: "+err.Error(),
			)
		}
		broker = reuse.Broker
	}
	if strings.TrimSpace(broker.ClientID) == "" {
		broker.ClientID = fmt.Sprintf("flowgo-probe-%d", time.Now().UnixNano()%1e9)
	}
	client, err := mqtt.Connect(broker, 10*time.Second)
	if err != nil {
		return failResult(locale, "连接失败: "+err.Error(), "connect failed: "+err.Error())
	}
	defer client.Disconnect(250)

	topic := cfg.Topic
	body := cfg.Payload
	if strings.TrimSpace(body) == "" {
		body = "{}"
	}
	token := client.Publish(topic, cfg.QoS, cfg.Retain, []byte(body))
	deadline := cfg.Timeout
	if deadline <= 0 {
		deadline = 10 * time.Second
	}
	if !token.WaitTimeout(deadline) {
		return failResult(locale, "发布超时: "+topic, "publish timeout: "+topic)
	}
	if err := token.Error(); err != nil {
		return failResult(locale, "发布失败: "+err.Error(), "publish failed: "+err.Error())
	}
	return okResult(locale,
		fmt.Sprintf("发布成功（topic=%s），已断开", topic),
		fmt.Sprintf("Published (topic=%s), disconnected", topic),
	)
}

func okResult(locale, zh, en string) ProbeResult {
	return ProbeResult{
		OK:      true,
		Message: types.PickI18n(map[string]string{types.LocaleEnUS: en}, locale, zh),
	}
}

func failResult(locale, zh, en string) ProbeResult {
	return ProbeResult{
		OK:      false,
		Message: types.PickI18n(map[string]string{types.LocaleEnUS: en}, locale, zh),
	}
}
