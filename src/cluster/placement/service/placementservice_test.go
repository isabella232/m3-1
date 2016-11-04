// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package service

import (
	"encoding/json"
	"errors"
	"math/rand"
	"sort"
	"testing"

	"github.com/m3db/m3cluster/placement"
	"github.com/stretchr/testify/assert"
)

func TestGoodWorkflow(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions(), NewMockStorage())
	testGoodWorkflow(t, ps)

	ps = NewPlacementService(placement.NewOptions().SetLooseRackCheck(true), NewMockStorage())
	testGoodWorkflow(t, ps)
}

func testGoodWorkflow(t *testing.T, ps placement.Service) {
	h1 := placement.NewHost("r1h1", "r1", "z1", 2)
	h2 := placement.NewHost("r2h2", "r2", "z1", 2)
	h3 := placement.NewHost("r3h3", "r3", "z1", 2)
	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{h1, h2}, 10, 1)
	assert.NoError(t, err)

	_, err = ps.AddReplica("serviceA")
	assert.NoError(t, err)

	_, err = ps.AddHost("serviceA", []placement.Host{h3})
	assert.NoError(t, err)

	_, err = ps.RemoveHost("serviceA", h1)
	assert.NoError(t, err)

	_, err = ps.ReplaceHost("serviceA",
		h2,
		[]placement.Host{
			placement.NewHost("h21", "r2", "z1", 1),
			placement.NewHost("h4", "r4", "z1", 1),
			h3, // already in placement
			placement.NewHost("h31", "r3", "z1", 1), // conflict
		},
	)
	assert.NoError(t, err)
	s, err := ps.Snapshot("serviceA")
	assert.NoError(t, err)
	assert.Equal(t, 3, s.HostsLen())
	assert.NotNil(t, s.HostShard("h21"))
	assert.NotNil(t, s.HostShard("h4"))

	_, err = ps.AddHost("serviceA", []placement.Host{h1})
	assert.NoError(t, err)

	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r2h4", "r2", "z1", 1)})
	assert.NoError(t, err)

	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r3h4", "r3", "z1", 1)})
	assert.NoError(t, err)
	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r3h5", "r3", "z1", 1)})
	assert.NoError(t, err)

	hosts := []placement.Host{
		placement.NewHost("r1h5", "r1", "z1", 1),
		placement.NewHost("r3h4", "r3", "z1", 1),
		placement.NewHost("r3h5", "r3", "z1", 1),
		placement.NewHost("r3h6", "r3", "z1", 1),
		placement.NewHost("r2h3", "r2", "z1", 1),
		placement.NewHost("r4h41", "r4", "z1", 1),
	}
	_, err = ps.AddHost("serviceA", hosts)
	assert.NoError(t, err)
	s, err = ps.Snapshot("serviceA")
	assert.NoError(t, err)
	assert.NotNil(t, s.HostShard("r4h41")) // host added from least weighted rack
}

func TestBadInitialPlacement(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions(), NewMockStorage())

	// no shards
	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{
		placement.NewHost("r1h1", "r1", "z1", 1),
		placement.NewHost("r2h2", "r2", "z1", 1),
	}, 0, 1)
	assert.Error(t, err)

	// not enough hosts
	_, err = ps.BuildInitialPlacement("serviceA", []placement.Host{}, 10, 1)
	assert.Error(t, err)

	// not enough racks
	_, err = ps.BuildInitialPlacement("serviceA", []placement.Host{
		placement.NewHost("r1h1", "r1", "z1", 1),
		placement.NewHost("r1h2", "r1", "z1", 1),
	}, 100, 2)
	assert.Error(t, err)

	// too many zones
	_, err = ps.BuildInitialPlacement("serviceA", []placement.Host{
		placement.NewHost("r1h1", "r1", "z1", 1),
		placement.NewHost("r2h2", "r2", "z2", 1),
	}, 100, 2)
	assert.Error(t, err)
	assert.Equal(t, errMultipleZones, err)

	_, err = ps.BuildInitialPlacement("serviceA", []placement.Host{
		placement.NewHost("r1h1", "r1", "z1", 1),
		placement.NewHost("r2h2", "r2", "z1", 1),
	}, 100, 2)
	assert.NoError(t, err)
}

func TestBadAddReplica(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions(), NewMockStorage())

	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{placement.NewHost("r1h1", "r1", "z1", 1)}, 10, 1)
	assert.NoError(t, err)

	// not enough racks/hosts
	_, err = ps.AddReplica("serviceA")
	assert.Error(t, err)

	// could not find snapshot for service
	_, err = ps.AddReplica("badService")
	assert.Error(t, err)
}

func TestBadAddHost(t *testing.T) {
	ms := NewMockStorage()
	ps := NewPlacementService(placement.NewOptions(), ms)

	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{placement.NewHost("r1h1", "r1", "z1", 1)}, 10, 1)
	assert.NoError(t, err)

	// adding host already exist
	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r1h1", "r1", "z1", 1)})
	assert.Error(t, err)

	// too many zones
	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r2h2", "r2", "z2", 1)})
	assert.Error(t, err)
	assert.Equal(t, errNoValidHost, err)

	// algo error
	psWithErrorAlgo := placementService{algo: errorAlgorithm{}, ss: ms, options: placement.NewOptions()}
	_, err = psWithErrorAlgo.AddHost("serviceA", []placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)})
	assert.Error(t, err)

	// could not find snapshot for service
	_, err = ps.AddHost("badService", []placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)})
	assert.Error(t, err)

	ps = NewPlacementService(placement.NewOptions(), ms)
	_, err = ps.AddHost("serviceA", []placement.Host{placement.NewHost("r1h1", "r1", "z1", 1)})
	assert.Error(t, err)
}

func TestBadRemoveHost(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions(), NewMockStorage())

	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{placement.NewHost("r1h1", "r1", "z1", 1)}, 10, 1)
	assert.NoError(t, err)

	// leaving host not exist
	_, err = ps.RemoveHost("serviceA", placement.NewHost("r2h2", "r2", "z1", 1))
	assert.Error(t, err)

	// not enough racks/hosts after removal
	_, err = ps.RemoveHost("serviceA", placement.NewHost("r1h1", "r1", "z1", 1))
	assert.Error(t, err)

	// could not find snapshot for service
	_, err = ps.RemoveHost("bad service", placement.NewHost("r1h1", "r1", "z1", 1))
	assert.Error(t, err)
}

func TestBadReplaceHost(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions(), NewMockStorage())

	_, err := ps.BuildInitialPlacement("serviceA", []placement.Host{
		placement.NewHost("r1h1", "r1", "z1", 1),
		placement.NewHost("r4h4", "r4", "z1", 1),
	}, 10, 1)
	assert.NoError(t, err)

	// leaving host not exist
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r1h2", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)},
	)
	assert.Error(t, err)

	// adding host already exist
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r1h1", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r4h4", "r4", "z1", 1)},
	)
	assert.Error(t, err)

	// not enough rack after replace
	_, err = ps.AddReplica("serviceA")
	assert.NoError(t, err)
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r4h4", "r4", "z1", 1),
		[]placement.Host{placement.NewHost("r1h2", "r1", "z1", 1)},
	)
	assert.Error(t, err)

	// could not find snapshot for service
	_, err = ps.ReplaceHost(
		"badService",
		placement.NewHost("r1h1", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)},
	)
	assert.Error(t, err)

	// catch algo errors
	psWithErrorAlgo := placementService{algo: errorAlgorithm{}, ss: NewMockStorage(), options: placement.NewOptions()}
	_, err = psWithErrorAlgo.ReplaceHost(
		"serviceA",
		placement.NewHost("r1h1", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)},
	)
	assert.Error(t, err)
}

func TestReplaceHostWithLooseRackCheck(t *testing.T) {
	ps := NewPlacementService(placement.NewOptions().SetLooseRackCheck(true), NewMockStorage())

	_, err := ps.BuildInitialPlacement(
		"serviceA",
		[]placement.Host{
			placement.NewHost("r1h1", "r1", "z1", 1),
			placement.NewHost("r4h4", "r4", "z1", 1),
		}, 10, 1)
	assert.NoError(t, err)

	// leaving host not exist
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r1h2", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)},
	)
	assert.Error(t, err)

	// adding host already exist
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r1h1", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r4h4", "r4", "z1", 1)},
	)
	assert.Error(t, err)

	// could not find snapshot for service
	_, err = ps.ReplaceHost(
		"badService",
		placement.NewHost("r1h1", "r1", "z1", 1),
		[]placement.Host{placement.NewHost("r2h2", "r2", "z1", 1)},
	)
	assert.Error(t, err)

	// NO ERROR when not enough rack after replace
	_, err = ps.AddReplica("serviceA")
	assert.NoError(t, err)
	_, err = ps.ReplaceHost(
		"serviceA",
		placement.NewHost("r4h4", "r4", "z1", 1),
		[]placement.Host{placement.NewHost("r1h2", "r1", "z1", 1)},
	)
	assert.NoError(t, err)
}

func TestFindReplaceHost(t *testing.T) {
	h1 := placement.NewHostShards(placement.NewHost("r1h1", "r11", "z1", 1))
	h1.AddShard(1)
	h1.AddShard(2)
	h1.AddShard(3)

	h10 := placement.NewHostShards(placement.NewHost("r1h10", "r11", "z1", 1))
	h10.AddShard(4)
	h10.AddShard(5)

	h2 := placement.NewHostShards(placement.NewHost("r2h2", "r12", "z1", 1))
	h2.AddShard(6)
	h2.AddShard(7)
	h2.AddShard(8)
	h2.AddShard(9)

	h3 := placement.NewHostShards(placement.NewHost("r3h3", "r13", "z1", 3))
	h3.AddShard(1)
	h3.AddShard(3)
	h3.AddShard(4)
	h3.AddShard(5)
	h3.AddShard(6)

	h4 := placement.NewHostShards(placement.NewHost("r4h4", "r14", "z1", 1))
	h4.AddShard(2)
	h4.AddShard(7)
	h4.AddShard(8)
	h4.AddShard(9)

	hss := []placement.HostShards{h1, h2, h3, h4, h10}

	ids := []uint32{1, 2, 3, 4, 5, 6, 7, 8}
	s := placement.NewPlacementSnapshot(hss, ids, 2)

	candidates := []placement.Host{
		placement.NewHost("h11", "r11", "z1", 1),
		placement.NewHost("h22", "r22", "z2", 1), // bad zone
	}

	ps := NewPlacementService(placement.NewOptions(), NewMockStorage()).(placementService)
	hs, err := ps.findReplaceHost(s, candidates, h4)
	assert.Error(t, err)
	assert.Nil(t, hs)

	noConflictCandidates := []placement.Host{
		placement.NewHost("h11", "r0", "z1", 1),
		placement.NewHost("h22", "r0", "z2", 1),
	}
	hs, err = ps.findReplaceHost(s, noConflictCandidates, h3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find enough host to replace")
	assert.Nil(t, hs)

	ps = NewPlacementService(placement.NewOptions().SetLooseRackCheck(true), NewMockStorage()).(placementService)
	hs, err = ps.findReplaceHost(s, candidates, h4)
	assert.NoError(t, err)
	// gonna prefer r1 because r1 would only conflict shard 2, r2 would conflict 7,8,9
	assert.Equal(t, 1, len(hs))
	assert.Equal(t, "r11", hs[0].Rack())
}

func TestGroupHostsByConflict(t *testing.T) {
	h1 := placement.NewHost("h1", "", "", 1)
	h2 := placement.NewHost("h2", "", "", 1)
	h3 := placement.NewHost("h3", "", "", 1)
	h4 := placement.NewHost("h4", "", "", 2)
	hostConflicts := []sortableValue{
		sortableValue{value: h1, weight: 1},
		sortableValue{value: h2, weight: 0},
		sortableValue{value: h3, weight: 3},
		sortableValue{value: h4, weight: 2},
	}

	groups := groupHostsByConflict(hostConflicts, true)
	assert.Equal(t, 4, len(groups))
	assert.Equal(t, h2, groups[0][0])
	assert.Equal(t, h1, groups[1][0])
	assert.Equal(t, h4, groups[2][0])
	assert.Equal(t, h3, groups[3][0])

	groups = groupHostsByConflict(hostConflicts, false)
	assert.Equal(t, 1, len(groups))
	assert.Equal(t, h2, groups[0][0])
}

func TestKnapSack(t *testing.T) {
	h1 := placement.NewHost("h1", "", "", 40000)
	h2 := placement.NewHost("h2", "", "", 20000)
	h3 := placement.NewHost("h3", "", "", 80000)
	h4 := placement.NewHost("h4", "", "", 50000)
	h5 := placement.NewHost("h5", "", "", 190000)
	hosts := []placement.Host{h1, h2, h3, h4, h5}

	res, leftWeight := knapsack(hosts, 10000)
	assert.Equal(t, -10000, leftWeight)
	assert.Equal(t, []placement.Host{h2}, res)

	res, leftWeight = knapsack(hosts, 20000)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h2}, res)

	res, leftWeight = knapsack(hosts, 30000)
	assert.Equal(t, -10000, leftWeight)
	assert.Equal(t, []placement.Host{h1}, res)

	res, leftWeight = knapsack(hosts, 60000)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2}, res)

	res, leftWeight = knapsack(hosts, 120000)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h3}, res)

	res, leftWeight = knapsack(hosts, 170000)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h3, h4}, res)

	res, leftWeight = knapsack(hosts, 190000)
	assert.Equal(t, 0, leftWeight)
	// will prefer h5 than h1+h2+h3+h4
	assert.Equal(t, []placement.Host{h5}, res)

	res, leftWeight = knapsack(hosts, 200000)
	assert.Equal(t, -10000, leftWeight)
	assert.Equal(t, []placement.Host{h2, h5}, res)

	res, leftWeight = knapsack(hosts, 210000)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h2, h5}, res)

	res, leftWeight = knapsack(hosts, 400000)
	assert.Equal(t, 20000, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5}, res)
}

func TestFillWeight(t *testing.T) {
	h1 := placement.NewHost("h1", "", "", 4)
	h2 := placement.NewHost("h2", "", "", 2)
	h3 := placement.NewHost("h3", "", "", 8)
	h4 := placement.NewHost("h4", "", "", 5)
	h5 := placement.NewHost("h5", "", "", 19)

	h6 := placement.NewHost("h6", "", "", 3)
	h7 := placement.NewHost("h7", "", "", 7)
	groups := [][]placement.Host{
		[]placement.Host{h1, h2, h3, h4, h5},
		[]placement.Host{h6, h7},
	}

	// When targetWeight is smaller than 38, the first group will satisfy
	res, leftWeight := fillWeight(groups, 1)
	assert.Equal(t, -1, leftWeight)
	assert.Equal(t, []placement.Host{h2}, res)

	res, leftWeight = fillWeight(groups, 2)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h2}, res)

	res, leftWeight = fillWeight(groups, 17)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h3, h4}, res)

	res, leftWeight = fillWeight(groups, 20)
	assert.Equal(t, -1, leftWeight)
	assert.Equal(t, []placement.Host{h2, h5}, res)

	// When targetWeight is bigger than 38, need to get host from group 2
	res, leftWeight = fillWeight(groups, 40)
	assert.Equal(t, -1, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5, h6}, res)

	res, leftWeight = fillWeight(groups, 41)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5, h6}, res)

	res, leftWeight = fillWeight(groups, 47)
	assert.Equal(t, -1, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5, h6, h7}, res)

	res, leftWeight = fillWeight(groups, 48)
	assert.Equal(t, 0, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5, h6, h7}, res)

	res, leftWeight = fillWeight(groups, 50)
	assert.Equal(t, 2, leftWeight)
	assert.Equal(t, []placement.Host{h1, h2, h3, h4, h5, h6, h7}, res)
}

func TestFillWeightDeterministic(t *testing.T) {
	h1 := placement.NewHost("h1", "", "", 1)
	h2 := placement.NewHost("h2", "", "", 1)
	h3 := placement.NewHost("h3", "", "", 1)
	h4 := placement.NewHost("h4", "", "", 3)
	h5 := placement.NewHost("h5", "", "", 4)

	h6 := placement.NewHost("h6", "", "", 1)
	h7 := placement.NewHost("h7", "", "", 1)
	h8 := placement.NewHost("h8", "", "", 1)
	h9 := placement.NewHost("h9", "", "", 2)
	groups := [][]placement.Host{
		[]placement.Host{h1, h2, h3, h4, h5},
		[]placement.Host{h6, h7, h8, h9},
	}

	for i := 1; i < 17; i++ {
		testResultDeterministic(t, groups, i)
	}
}

func testResultDeterministic(t *testing.T, groups [][]placement.Host, targetWeight int) {
	res, _ := fillWeight(groups, targetWeight)

	// shuffle the order of of each group of hosts
	for _, group := range groups {
		for i := range group {
			j := rand.Intn(i + 1)
			group[i], group[j] = group[j], group[i]
		}
	}
	res1, _ := fillWeight(groups, targetWeight)
	assert.Equal(t, res, res1)
}

func TestRackLenSort(t *testing.T) {
	r1 := sortableValue{value: "r1", weight: 1}
	r2 := sortableValue{value: "r2", weight: 2}
	r3 := sortableValue{value: "r3", weight: 3}
	r4 := sortableValue{value: "r4", weight: 2}
	r5 := sortableValue{value: "r5", weight: 1}
	r6 := sortableValue{value: "r6", weight: 2}
	r7 := sortableValue{value: "r7", weight: 3}
	rs := sortableThings{r1, r2, r3, r4, r5, r6, r7}
	sort.Sort(rs)

	seen := 0
	for _, rl := range rs {
		assert.True(t, seen <= rl.weight)
		seen = rl.weight
	}
}

type errorAlgorithm struct{}

func (errorAlgorithm) BuildInitialPlacement(hosts []placement.Host, ids []uint32) (placement.Snapshot, error) {
	return nil, errors.New("error in errorAlgorithm")
}

func (errorAlgorithm) AddReplica(p placement.Snapshot) (placement.Snapshot, error) {
	return nil, errors.New("error in errorAlgorithm")
}

func (errorAlgorithm) AddHost(p placement.Snapshot, h placement.Host) (placement.Snapshot, error) {
	return nil, errors.New("error in errorAlgorithm")
}

func (errorAlgorithm) RemoveHost(p placement.Snapshot, h placement.Host) (placement.Snapshot, error) {
	return nil, errors.New("error in errorAlgorithm")
}

func (errorAlgorithm) ReplaceHost(p placement.Snapshot, leavingHost placement.Host, addingHost []placement.Host) (placement.Snapshot, error) {
	return nil, errors.New("error in errorAlgorithm")
}

// file based snapshot storage
type mockStorage struct {
	m map[string][]byte
}

const configFileSuffix = "_placement.json"

func getSnapshotFileName(service string) string {
	return service + configFileSuffix
}

func NewMockStorage() placement.SnapshotStorage {
	return &mockStorage{m: map[string][]byte{}}
}

func (ms *mockStorage) SaveSnapshotForService(service string, p placement.Snapshot) error {
	var err error
	if err = p.Validate(); err != nil {
		return err
	}
	var data []byte
	if data, err = json.Marshal(p); err != nil {
		return err
	}
	ms.m[service] = data
	return nil
}

func (ms *mockStorage) ReadSnapshotForService(service string) (placement.Snapshot, error) {
	if data, exist := ms.m[service]; exist {
		return placement.NewPlacementFromJSON(data)
	}
	return nil, errors.New("could not find snapshot for service")
}