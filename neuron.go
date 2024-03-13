package zenmodel

import (
	"encoding/json"
	"fmt"

	"github.com/zenmodel/zenmodel/utils"
)

const (
	DefaultConductGroupName = "default"
)

var (
	defaultSelectFn = func(brain Brain) string {
		return DefaultConductGroupName
	}
)

func newNeuron() *Neuron {
	return &Neuron{
		id:    utils.GenUUID(),
		state: NeuronStateInhibited,
		conductGroups: map[string]ConductGroup{
			DefaultConductGroupName: map[string]bool{},
		},
		triggerGroups: map[string]TriggerGroup{},
		selectFn:      defaultSelectFn,
		labels:        map[string]string{},
	}
}

type Neuron struct {
	// ID 不可编辑
	id string
	// state 不可编辑，
	state NeuronState
	count struct {
		process int
		succeed int
		failed  int
	}

	processor Processor

	// 触发组,触发组是用来控制神经元的触发条件
	triggerGroups TriggerGroups
	// 传导组,传导组是用来控制神经元之间的传导关系
	conductGroups ConductGroups
	// 在 neuron 运行成功之后通过 selectFn 决定传导到哪一个传导组
	selectFn func(brain Brain) string

	// 便于二次开发的扩展
	labels map[string]string
	stopCh chan bool
}

type TriggerGroups map[string]TriggerGroup

// TriggerGroup link IDs
type TriggerGroup []string

// ConductGroups map of conduct group name and ConductGroup
type ConductGroups map[string]ConductGroup

// ConductGroup map[linkID]true
type ConductGroup map[string]bool

type NeuronState string

const (
	NeuronStateInhibited NeuronState = "Inhibited"
	NeuronStateActivated NeuronState = "Activated"
)

func (n *Neuron) DeepCopy() *Neuron {
	if n == nil {
		return nil
	}
	cp := &Neuron{
		id:            n.id,
		state:         n.state,
		processor:     n.processor.DeepCopy(),
		selectFn:      n.selectFn,
		triggerGroups: make(TriggerGroups),
		conductGroups: make(ConductGroups),
		labels:        make(map[string]string),
	}
	cp.count.process = n.count.process
	cp.count.succeed = n.count.succeed
	cp.count.failed = n.count.failed
	for tgName, tg := range n.triggerGroups {
		newTg := make(TriggerGroup, len(tg))
		copy(newTg, tg)
		cp.triggerGroups[tgName] = newTg
	}
	for cgName, cg := range n.conductGroups {
		newCg := make(ConductGroup)
		for linkId, val := range cg {
			newCg[linkId] = val
		}
		cp.conductGroups[cgName] = newCg
	}
	for label, value := range n.labels {
		cp.labels[label] = value
	}
	return cp
}

func (n *Neuron) GetID() string {
	return n.id
}

func (n *Neuron) SetLabels(l map[string]string) {
	n.labels = l
}

func (n *Neuron) addLinkToDefaultConductGroup(links ...*Link) error {
	return n.addLinkToConductGroup(DefaultConductGroupName, links...)
}

// addLinkToConductGroup 把指定 links 加到指定 group, group 如果不存在则新建 group
// 指定 link 如果原本属于 default group，则先从 default group 中移除
// 指定 link 如果原本属于 其他非 default group，不会从其他group 中移除
// 允许添加空 link 的组
// TODO 增加线程安全， add lock
func (n *Neuron) addLinkToConductGroup(groupName string, links ...*Link) error {
	// init
	if n.conductGroups == nil {
		n.conductGroups = map[string]ConductGroup{
			DefaultConductGroupName: map[string]bool{},
		}
	}
	if n.conductGroups[DefaultConductGroupName] == nil {
		n.conductGroups[DefaultConductGroupName] = map[string]bool{}
	}
	if n.conductGroups[groupName] == nil {
		n.conductGroups[groupName] = map[string]bool{}
	}

	for _, link := range links {
		// 检查是否是当前节点的导出连接
		if link.from != n.id {
			return fmt.Errorf("link %s not from neuron %s", link.id, n.id)
		}
		// 指定 link 如果原本属于 default group，则先从 default group 中移除
		if n.conductGroups[DefaultConductGroupName][link.id] {
			delete(n.conductGroups[DefaultConductGroupName], link.id)
		}
		// add link to group
		n.conductGroups[groupName][link.id] = true
	}

	return nil
}

// deleteConductGroup 删除一个组中的所有 link, 并且删除组
// 不能通过此方法删除 default group, 删除 group 后，孤立的 out link 会被再次加入 default group
// TODO 增加线程安全， add lock
func (n *Neuron) deleteConductGroup(groupName string) error {
	if groupName == DefaultConductGroupName {
		return fmt.Errorf("cannot delete default conduct group")
	}
	if n.conductGroups[DefaultConductGroupName] == nil {
		n.conductGroups[DefaultConductGroupName] = map[string]bool{}
	}
	if len(n.conductGroups[groupName]) == 0 { // 组不存在，或已经是空组
		delete(n.conductGroups, groupName)
	}

	ungroupLinks := []string{}
	for linkID, _ := range n.conductGroups[groupName] {
		ungroupLinks = append(ungroupLinks, linkID)
	}
	// delete
	delete(n.conductGroups, groupName)

	for _, ungroupLink := range ungroupLinks {
		// check if link isolate(not contains in other group), add to default group
		if !n.conductGroups.containsLink(ungroupLink) {
			n.conductGroups[DefaultConductGroupName][ungroupLink] = true
		}
	}

	return nil
}

func (cgs ConductGroups) containsLink(linkID string) bool {
	for _, group := range cgs {
		for curLinkID, _ := range group {
			if linkID == curLinkID {
				return true
			}
		}
	}

	return false
}

// AddTriggerGroup 默认情况下单个传入边自己一条边一组。AddTriggerGroup 将指定指定传入连接加到同一触发组。
// AddTriggerGroup 增加触发组，如果增加的触发组包含了存量的触发组，则存量的触发组会移除。这样同时也可以去重
// TODO 增加线程安全， add lock
// add trigger group with links
func (n *Neuron) addTriggerGroup(links ...*Link) error {
	if len(links) == 0 {
		return nil
	}
	newGroup := []string{}
	for _, link := range links {
		// 检查是否是当前节点的传入连接
		if link.to != n.id {
			return fmt.Errorf("link %s not to neuron %s", link.id, n.id)
		}
		newGroup = append(newGroup, link.id)
	}

	for key, group := range n.triggerGroups {
		// 确保要增加的触发组没有被存量的组包含
		if utils.SlicesContains(group, newGroup) {
			return fmt.Errorf("has group %s contains want add group", group)
		}
		// 确保要增加的触发组没有包含存量的组，有则移除存量的组
		if utils.SlicesContains(newGroup, group) {
			delete(n.triggerGroups, key)
		}
	}

	// add
	n.triggerGroups[utils.GenUUID()] = newGroup

	return nil
}

// TODO 增加线程安全， add lock
func (n *Neuron) deleteTriggerGroup(links ...*Link) {
	deleteGroup := []string{}
	for _, link := range links {
		deleteGroup = append(deleteGroup, link.id)
	}

	for key, group := range n.triggerGroups {
		if utils.SlicesContainEqual(group, deleteGroup) {
			delete(n.triggerGroups, key)
		}
	}
}

func (n *Neuron) bindProcessor(p Processor) {
	n.processor = p
}

func (n *Neuron) String() string {
	ts, _ := json.Marshal(n.triggerGroups)
	cs, _ := json.Marshal(n.conductGroups)
	ls, _ := json.Marshal(n.labels)
	return fmt.Sprintf(`{
	"id": "%s",
	"state": "%s",
	"trigger_groups": %s,
	"conduct_groups": %s,
	"labels": %s
}`, n.id, n.state, ts, cs, ls)
}