package gcore

import (
	"errors"
	"math"
	"sort"
	"sync"
)

// Position 表示游戏世界中的二维坐标。
type Position struct {
	X float64
	Y float64
}

// AOICell 表示 AOI 网格中的一个格子坐标。
type AOICell struct {
	X int
	Y int
}

// AOIChange 表示一个实体移动后，视野中新增和离开的实体。
type AOIChange struct {
	Entered []uint32
	Left    []uint32
}

type aoiEntity struct {
	id       uint32
	position Position
	cell     AOICell
}

// AOIManager 使用二维网格管理实体的邻近关系。
//
// 当前实现查询实体所在格子及其周围八个格子，适合规则地图和固定视野范围。
// 如果游戏需要圆形视野或不同实体拥有不同视野半径，应在业务层继续过滤查询结果。
type AOIManager struct {
	worldWidth  float64
	worldHeight float64
	cellWidth   float64
	cellHeight  float64

	entities map[uint32]*aoiEntity
	cells    map[AOICell]map[uint32]struct{}
	lock     sync.RWMutex
}

// NewAOIManager 创建一个从坐标原点开始的二维 AOI 网格。
func NewAOIManager(worldWidth, worldHeight, cellWidth, cellHeight float64) (*AOIManager, error) {
	if worldWidth <= 0 || worldHeight <= 0 {
		return nil, errors.New("aoi world size must be positive")
	}
	if cellWidth <= 0 || cellHeight <= 0 {
		return nil, errors.New("aoi cell size must be positive")
	}

	return &AOIManager{
		worldWidth:  worldWidth,
		worldHeight: worldHeight,
		cellWidth:   cellWidth,
		cellHeight:  cellHeight,
		entities:    make(map[uint32]*aoiEntity),
		cells:       make(map[AOICell]map[uint32]struct{}),
	}, nil
}

// AddEntity 将一个实体加入 AOI 管理器。
func (am *AOIManager) AddEntity(entityID uint32, position Position) error {
	am.lock.Lock()
	defer am.lock.Unlock()

	if _, ok := am.entities[entityID]; ok {
		return errors.New("aoi entity already exists")
	}
	cell, err := am.cellFor(position)
	if err != nil {
		return err
	}

	am.entities[entityID] = &aoiEntity{
		id:       entityID,
		position: position,
		cell:     cell,
	}
	am.addToCell(cell, entityID)
	return nil
}

// RemoveEntity 将一个实体从 AOI 管理器中移除。
func (am *AOIManager) RemoveEntity(entityID uint32) error {
	am.lock.Lock()
	defer am.lock.Unlock()

	entity, ok := am.entities[entityID]
	if !ok {
		return errors.New("aoi entity not found")
	}
	am.removeFromCell(entity.cell, entityID)
	delete(am.entities, entityID)
	return nil
}

// MoveEntity 更新实体坐标，并返回视野新增和离开的实体。
func (am *AOIManager) MoveEntity(entityID uint32, position Position) (AOIChange, error) {
	am.lock.Lock()
	defer am.lock.Unlock()

	entity, ok := am.entities[entityID]
	if !ok {
		return AOIChange{}, errors.New("aoi entity not found")
	}
	newCell, err := am.cellFor(position)
	if err != nil {
		return AOIChange{}, err
	}

	oldNearby := am.nearbyLocked(entity)
	if entity.cell != newCell {
		am.removeFromCell(entity.cell, entityID)
		am.addToCell(newCell, entityID)
		entity.cell = newCell
	}
	entity.position = position
	newNearby := am.nearbyLocked(entity)

	return AOIChange{
		Entered: difference(newNearby, oldNearby),
		Left:    difference(oldNearby, newNearby),
	}, nil
}

// GetPosition 获取实体当前坐标。
func (am *AOIManager) GetPosition(entityID uint32) (Position, error) {
	am.lock.RLock()
	defer am.lock.RUnlock()

	entity, ok := am.entities[entityID]
	if !ok {
		return Position{}, errors.New("aoi entity not found")
	}
	return entity.position, nil
}

// GetNearbyEntityIDs 获取实体所在格子周围九宫格中的其他实体。
func (am *AOIManager) GetNearbyEntityIDs(entityID uint32) ([]uint32, error) {
	am.lock.RLock()
	defer am.lock.RUnlock()

	entity, ok := am.entities[entityID]
	if !ok {
		return nil, errors.New("aoi entity not found")
	}
	return am.nearbyLocked(entity), nil
}

// CellFor 获取坐标对应的网格格子，便于业务层做调试和统计。
func (am *AOIManager) CellFor(position Position) (AOICell, error) {
	am.lock.RLock()
	defer am.lock.RUnlock()
	return am.cellFor(position)
}

func (am *AOIManager) cellFor(position Position) (AOICell, error) {
	if math.IsNaN(position.X) || math.IsNaN(position.Y) || math.IsInf(position.X, 0) || math.IsInf(position.Y, 0) {
		return AOICell{}, errors.New("aoi position is not finite")
	}
	if position.X < 0 || position.Y < 0 || position.X >= am.worldWidth || position.Y >= am.worldHeight {
		return AOICell{}, errors.New("aoi position is outside world")
	}
	return AOICell{
		X: int(math.Floor(position.X / am.cellWidth)),
		Y: int(math.Floor(position.Y / am.cellHeight)),
	}, nil
}

func (am *AOIManager) addToCell(cell AOICell, entityID uint32) {
	if am.cells[cell] == nil {
		am.cells[cell] = make(map[uint32]struct{})
	}
	am.cells[cell][entityID] = struct{}{}
}

func (am *AOIManager) removeFromCell(cell AOICell, entityID uint32) {
	entities := am.cells[cell]
	delete(entities, entityID)
	if len(entities) == 0 {
		delete(am.cells, cell)
	}
}

func (am *AOIManager) nearbyLocked(entity *aoiEntity) []uint32 {
	nearby := make([]uint32, 0)
	for x := entity.cell.X - 1; x <= entity.cell.X+1; x++ {
		for y := entity.cell.Y - 1; y <= entity.cell.Y+1; y++ {
			for entityID := range am.cells[AOICell{X: x, Y: y}] {
				if entityID != entity.id {
					nearby = append(nearby, entityID)
				}
			}
		}
	}
	sort.Slice(nearby, func(i, j int) bool { return nearby[i] < nearby[j] })
	return nearby
}

func difference(source, target []uint32) []uint32 {
	contains := make(map[uint32]struct{}, len(target))
	for _, entityID := range target {
		contains[entityID] = struct{}{}
	}
	result := make([]uint32, 0)
	for _, entityID := range source {
		if _, ok := contains[entityID]; !ok {
			result = append(result, entityID)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
