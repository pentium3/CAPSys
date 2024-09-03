package caps_single

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
)

// Read job graph Json file and generate operator list
func ReadJsonJG(jobGraph_jsonFile string) *[]*Operator {
	res := []*Operator{}

	// Read json file
	bytes, err := os.ReadFile(jobGraph_jsonFile)
	if err != nil {
		fmt.Println("Convert json format Error reading file:", err)
		os.Exit(1)
	}
	// Decode the json object
	var ops []map[string]interface{}
	err = json.Unmarshal(bytes, &ops)
	if err != nil {
		fmt.Println("Convert json format Error unmarshalling JSON:", err)
		os.Exit(1)
	}
	// Convert data type
	for _, op := range ops {
		// convert id to byte
		id_str, ok := op["id"].(string)
		if !ok {
			fmt.Println("Convert json format Error: Expected a string")
			os.Exit(1)
		}
		// convert name to string
		name_str, ok := op["name"].(string)
		if !ok {
			fmt.Println("Convert json format Error: Expected a string")
			os.Exit(1)
		}
		// convert parallelism to int
		para_float, ok := op["parallelism"].(float64)
		if !ok {
			fmt.Println("Convert json format Error: The value is not a number")
			os.Exit(1)
		}
		// convert upNode list to []byte
		slice, ok := op["upNode"].([]interface{})
		if !ok {
			fmt.Println("Convert json format Error: Expected a slice of interfaces")
			os.Exit(1)
		}
		upNode := []byte{}
		for _, v := range slice {
			str, ok := v.(string)
			if !ok {
				fmt.Println("Convert json format Error: Expected a string")
				os.Exit(1)
			}
			upNode = append(upNode, str[0])
		}
		// convert downNode list to []byte
		slice, ok = op["downNode"].([]interface{})
		if !ok {
			fmt.Println("Convert json format Error: Expected a slice of interfaces")
			os.Exit(1)
		}
		downNode := []byte{}
		for _, v := range slice {
			str, ok := v.(string)
			if !ok {
				fmt.Println("Convert json format Error: Expected a string")
				os.Exit(1)
			}
			downNode = append(downNode, str[0])
		}
		// convert outboundtype to string
		outboundtype_str, ok := op["outboundtype"].(string)
		if !ok {
			fmt.Println("Convert json format Error: Expected a string")
			os.Exit(1)
		}
		// convert cost values to float
		compute_float, ok := op["compute"].(float64)
		if !ok {
			fmt.Println("Convert json format Error: The value is not a number")
			os.Exit(1)
		}
		network_float, ok := op["network"].(float64)
		if !ok {
			fmt.Println("Convert json format Error: The value is not a number")
			os.Exit(1)
		}
		state_float, ok := op["state"].(float64)
		if !ok {
			fmt.Println("Convert json format Error: The value is not a number")
			os.Exit(1)
		}

		// Init an Operator object and append it to the list
		newOp := Operator{
			Id:           id_str[0],
			Name:         name_str,
			Parallelism:  int(para_float),
			DownNodes:    downNode,
			UpNodes:      upNode,
			OutboundType: outboundtype_str,
			ResRequirement: &Resource{
				Compute: compute_float,
				State:   state_float,
				Network: network_float,
			},
		}
		res = append(res, &newOp)
	}
	return &res
}

// Sort the outer search operator stack
func SortOuterSearch(ops *[]*Operator, outer_dfs_order string) *[]*Operator {
	// TODO: check outer_dfs_order input

	// TODO: sort ops based on outer_dfs_order

	return ops
}

// Check the correctness of operator info
func CheckOPs(ops_pointer *[]*Operator, workerNum int, slotNum int) {
	ops := *ops_pointer
	if len(ops) < 2 {
		fmt.Println("Invalid compute graph: need at least 2 operators")
		os.Exit(1)
	}
	totalSlots := workerNum * slotNum
	submittedTaskNum := 0
	for _, op := range ops {
		submittedTaskNum += op.Parallelism
		if op.ResRequirement.Compute <= 0 || op.ResRequirement.Network < 0 || op.ResRequirement.State < 0 {
			fmt.Println("Invalid operator cost")
			os.Exit(1)
		}
	}
	if submittedTaskNum > totalSlots {
		fmt.Println("Number of submitted tasks > Number of available slots")
		os.Exit(1)
	}
}

// Check the correctness of input threshold ratio
func CheckThresholdRatio(threshold_ratio *ThresholdRatio) {
	if !(threshold_ratio.Compute >= 0 && threshold_ratio.Compute <= 1) {
		fmt.Println("[Input Error] compute ratio should be in [0, 1]")
		os.Exit(1)
	}
	if !(threshold_ratio.State >= 0 && threshold_ratio.State <= 1) {
		fmt.Println("[Input Error] state ratio should be in [0, 1]")
		os.Exit(1)
	}
	if !(threshold_ratio.Network >= 0 && threshold_ratio.Network <= 1) {
		fmt.Println("[Input Error] network ratio should be in [0, 1])")
		os.Exit(1)
	}
}

// Calculate the cost threshold given the threshold ratio
func CalculateThreshold(ops_pointer *[]*Operator, workerNum int, slotNum int, threshold_ratio *ThresholdRatio) (float64, float64, float64) {
	// get min cost for compute/state
	total_compute := float64(0)
	total_state := float64(0)
	ops := *ops_pointer
	tempOpList := []*OpCostTuple{} // for calculating max cost
	for _, op := range ops {
		total_compute += op.ResRequirement.Compute * float64(op.Parallelism)
		total_state += op.ResRequirement.State * float64(op.Parallelism)
		tempOpList = append(tempOpList, &OpCostTuple{
			Parallelism: op.Parallelism,
			ComputeCost: op.ResRequirement.Compute,
			StateCost:   op.ResRequirement.State,
			NetworkCost: op.ResRequirement.Network,
		})
	}
	min_compute_cost := total_compute / float64(workerNum)
	min_state_cost := total_state / float64(workerNum)
	min_network_cost := float64(0)

	// Get upper bound cost for compute
	sort.Slice(tempOpList, func(i, j int) bool {
		return tempOpList[i].ComputeCost > tempOpList[j].ComputeCost
	})
	max_compute_cost := GetUpperBoundForCost(tempOpList, slotNum, "compute")

	// Get upper bound cost for state
	sort.Slice(tempOpList, func(i, j int) bool {
		return tempOpList[i].StateCost > tempOpList[j].StateCost
	})
	max_state_cost := GetUpperBoundForCost(tempOpList, slotNum, "state")

	// Get upper bound cost for network
	sort.Slice(tempOpList, func(i, j int) bool {
		return tempOpList[i].NetworkCost > tempOpList[j].NetworkCost
	})
	max_network_cost := GetUpperBoundForCost(tempOpList, slotNum, "network")

	// Calculate threshold for all dimensions
	threshold_compute := min_compute_cost + (max_compute_cost-min_compute_cost)*threshold_ratio.Compute
	threshold_state := min_state_cost + (max_state_cost-min_state_cost)*threshold_ratio.State
	threshold_network := min_network_cost + (max_network_cost-min_network_cost)*threshold_ratio.Network

	// Round the threshold
	// Round all cost values to only keep 5 decimal digits
	scaleFactor := math.Pow(10, 5)
	threshold_compute = math.Round(threshold_compute*scaleFactor) / scaleFactor
	threshold_state = math.Round(threshold_state*scaleFactor) / scaleFactor
	threshold_network = math.Round(threshold_network*scaleFactor) / scaleFactor

	// // fmt.Println("======== Cost threshold ========")
	// fmt.Println("Compute threshold:", threshold_compute, "Min:", min_compute_cost, "Max:", max_compute_cost)
	// fmt.Println("State threshold:", threshold_state, "Min:", min_state_cost, "Max:", max_state_cost)
	// fmt.Println("Network threshold:", threshold_network, "Min:", min_network_cost, "Max:", max_network_cost)

	return threshold_compute, threshold_state, threshold_network
}

// Get the max possible cost for different dimensions
func GetUpperBoundForCost(tempOpList []*OpCostTuple, slotNum int, whichCost string) float64 {
	max_cost := float64(0)
	var numTaskToDeploy int
	for _, op := range tempOpList {
		if slotNum < op.Parallelism {
			numTaskToDeploy = slotNum
		} else {
			numTaskToDeploy = op.Parallelism
		}
		if whichCost == "compute" {
			max_cost += op.ComputeCost * float64(numTaskToDeploy)
		} else if whichCost == "state" {
			max_cost += op.StateCost * float64(numTaskToDeploy)
		} else {
			max_cost += op.NetworkCost * float64(numTaskToDeploy)
		}
		slotNum -= numTaskToDeploy
		if slotNum <= 0 {
			break
		}
	}
	return max_cost
}

// Initiate the reference map for caps
func InitOpsReferenceMap(ops *[]*Operator) map[byte]*Operator {
	res := make(map[byte]*Operator)
	for _, op := range *ops {
		res[op.Id] = op
	}
	return res
}

// Task placement on a node is identified by a []byte or a equivalent string. Convert the byte array to a string
func ConvertTaskArrayToStr(tasks []byte) string {
	// Sort the task list first
	sort.Slice(tasks, func(i, j int) bool { return tasks[i] > tasks[j] })
	return string(tasks)
}

// Convert a string back to the byte array
func ConvertTaskStrToArray(tasks_str string) []byte {
	return []byte(tasks_str)
}

// Init CurPlacement struct
func NewCurPlacement(workerNum int, slotNum int) *CurPlacement {
	initialMap := make(map[string]*NodePlacementInfo)
	// Initialize the map
	initialMap[""] = &NodePlacementInfo{
		Count: workerNum,
		ResUsage: &Resource{
			Compute: 0,
			Network: 0,
			State:   0,
		},
	}
	return &CurPlacement{
		LeftSlots: workerNum * slotNum,
		Map:       initialMap,
	}
}

///////////////////////////// CAPS methods /////////////////////////////

// Pop out an operator from the stack (at each layer of the outer search)
func (caps *CAPS) Pop_from_Ops() *Operator {
	ops_p := caps.Ops
	op := (*ops_p)[len(*ops_p)-1]
	*ops_p = (*ops_p)[:len(*ops_p)-1]
	return op
}

// Push the operator back to the stack (backtrace outer search)
func (caps *CAPS) Push_to_Ops(op *Operator) {
	ops_p := caps.Ops
	*ops_p = append(*ops_p, op)
}

// Convert the CurPlace to a final placement plan
func (caps *CAPS) GeneratePlan() *Plan {
	placement := []string{}
	cost := Resource{
		Compute: 0,
		State:   0,
		Network: 0,
	}
	for key, value := range caps.CurPlace.Map {
		if key != "" {
			for i := 0; i < value.Count; i++ {
				placement = append(placement, key)
			}
			// Use max cost as the plan cost
			if value.ResUsage.Compute > cost.Compute {
				cost.Compute = value.ResUsage.Compute
			}
			if value.ResUsage.State > cost.State {
				cost.State = value.ResUsage.State
			}
			if value.ResUsage.Network > cost.Network {
				cost.Network = value.ResUsage.Network
			}
		}
	}
	// Round all cost values to only keep 2 decimal digits
	scaleFactor := math.Pow(10, 2)
	cost.Compute = math.Round(cost.Compute*scaleFactor) / scaleFactor
	cost.State = math.Round(cost.State*scaleFactor) / scaleFactor
	cost.Network = math.Round(cost.Network*scaleFactor) / scaleFactor
	return &Plan{
		Placement: placement,
		Cost:      &cost,
	}
}

// Update CurPlace at the end of each inner search procedure
func (caps *CAPS) UpdateCurPlace(nodes *[]*Node, op *Operator) *CurPlacement {
	// Some error check
	if len(*nodes) != caps.WorkerNum {
		fmt.Println("nodes list has incorrect # of workers")
		os.Exit(1)
	}
	// Convert the nodes list to a new CurPlace
	newMap := make(map[string]*NodePlacementInfo)
	for _, node := range *nodes {
		newList := ConvertTaskStrToArray(node.Key)
		for i := 0; i < node.DeploymentCount; i++ {
			newList = append(newList, op.Id)
		}
		newKey := ConvertTaskArrayToStr(newList)
		if _, ok := newMap[newKey]; ok {
			newMap[newKey].Count += 1
		} else {
			newMap[newKey] = &NodePlacementInfo{
				Count:    1,
				ResUsage: node.ResUsage,
			}
		}
	}
	// Construct the new CurPlacement and return the pointer to the old one for backtrace purposes
	res := caps.CurPlace
	caps.CurPlace = &CurPlacement{
		LeftSlots: res.LeftSlots - op.Parallelism,
		Map:       newMap,
	}
	return res
}

// Backtrace the CurPlace
func (caps *CAPS) BacktraceCurPlace(oldCurPlace *CurPlacement) {
	caps.CurPlace = oldCurPlace
}

// Copy CurPlace to get a snapshot of current task placements for an inner search procedure
func (caps *CAPS) BuildNodeList() *[]*Node {
	res := []*Node{} // The length of nodes is the total num of workers in the cluster
	for key, value := range caps.CurPlace.Map {
		for i := 0; i < value.Count; i++ {
			res = append(res, &Node{
				Key: key,
				ResUsage: &Resource{ // deep copy
					Compute: value.ResUsage.Compute,
					Network: value.ResUsage.Network,
					State:   value.ResUsage.State,
				},
				DeploymentCount: 0,
			})
		}
	}
	return &res
}

// Update a node in NodeList at each layer of the inner search
func (caps *CAPS) UpdateNode(node *Node, op *Operator, num int) float64 {
	// init nwcost caused by downstream link
	localnwcost := float64(0)
	if num == 0 {
		return localnwcost
	}
	// Update node compute/state cost
	node.DeploymentCount = num
	node.ResUsage.Compute += op.ResRequirement.Compute * float64(num)
	node.ResUsage.State += op.ResRequirement.State * float64(num)
	// Update node network cost: only consider downstream cost here (upstream cost is handled at the end of each OP)
	downNodeList := op.DownNodes
	if len(downNodeList) > 1 {
		fmt.Println("Operator has multiple downstream operators: query type temporarly not supported now (couldn't calculate network cost)")
		os.Exit(1)
	}
	for _, downOp := range downNodeList { // only 1 in the list
		if _, ok := caps.DeployedOps[downOp]; ok {
			downOpPara := caps.OpsMap[downOp].Parallelism
			// Check how many downOp tasks are colocated on this node
			numDownOpDeployed := 0
			for _, task := range ConvertTaskStrToArray(node.Key) {
				if task == downOp {
					numDownOpDeployed += 1
				}
			}
			if op.OutboundType == "FORWARD" {
				remote_link_count := num - numDownOpDeployed
				if remote_link_count < 0 {
					remote_link_count = 0
				}
				localnwcost += op.ResRequirement.Network * float64(remote_link_count)
			} else {
				localnwcost += (float64(1) - float64(numDownOpDeployed)/float64(downOpPara)) * op.ResRequirement.Network * float64(num)
			}
		}
	}
	node.ResUsage.Network += localnwcost
	// Return localnwcost for backtrace purposes
	return localnwcost
}

// Backtrace the node
func (caps *CAPS) BacktraceNode(node *Node, op *Operator, num int, addedlocalnwcost float64) {
	if num == 0 {
		return
	}
	node.DeploymentCount = 0
	node.ResUsage.Compute -= op.ResRequirement.Compute * float64(num)
	node.ResUsage.State -= op.ResRequirement.State * float64(num)
	node.ResUsage.Network -= addedlocalnwcost
}

// Get pareto-optimal plans from the resulting plan list (Naive O(n^2) solution)
func (caps *CAPS) ParetoOptimalPlans() []*Plan {
	res := []*Plan{}
	if len(*caps.Plans) == 0 {
		return res
	}
outerLoop:
	for _, plan := range *caps.Plans {
		for _, other_plan := range *caps.Plans {
			if plan == other_plan {
				continue
			}
			if plan.Cost.IsDominatedBy(other_plan.Cost) {
				continue outerLoop
			}
		}
		res = append(res, plan)
	}
	return res
}

// Print out the detail of a placement plan
func (caps *CAPS) PrintOutPlan(plan *Plan) {
	reference := caps.OpsMap
	fmt.Println("=========== Plan Details ===========")
	for i, nodeTasks := range plan.Placement {
		fmt.Printf("Node %d: ", i+1)
		for _, task := range ConvertTaskStrToArray(nodeTasks) {
			fmt.Printf("%s ", reference[task].Name)
		}
		fmt.Println()
	}
	fmt.Printf("Plan cost: compute %f, state %f, network %f \n", plan.Cost.Compute, plan.Cost.State, plan.Cost.Network)
	fmt.Println()
}

// Print out the plan cost
func (caps *CAPS) PrintOutPlanCost(plan *Plan) {
	fmt.Printf("compute %f state %f network %f\n", plan.Cost.Compute, plan.Cost.State, plan.Cost.Network)
}

// Verify the plan cost obtained from CAPS, compared to an independent manual calculation
func (caps *CAPS) TestCostCorrectnessForAllPlans() {
	// Check each plan
	for _, plan := range *caps.Plans {
		// Cost from manual calculation
		planCost := Resource{
			Compute: 0,
			State:   0,
			Network: 0,
		}
		// Check each node in this plan
		for _, nodePlace := range plan.Placement {
			// Cost of this node
			cpcost := float64(0)
			iocost := float64(0)
			nwcost := float64(0)
			// Get compute and state cost
			taskCounts := make(map[byte]int)
			for _, task := range ConvertTaskStrToArray(nodePlace) {
				cpcost += caps.OpsMap[task].ResRequirement.Compute
				iocost += caps.OpsMap[task].ResRequirement.State
				taskCounts[task]++
			}
			// Get network cost
			for task, num := range taskCounts {
				downOps := caps.OpsMap[task].DownNodes
				for _, downOp := range downOps {
					downOpNum := taskCounts[downOp]
					downOpPara := caps.OpsMap[downOp].Parallelism
					if caps.OpsMap[task].OutboundType == "FORWARD" {
						remote_link_count := num - downOpNum
						if remote_link_count < 0 {
							remote_link_count = 0
						}
						nwcost += float64(remote_link_count) * caps.OpsMap[task].ResRequirement.Network
					} else {
						nwcost += (float64(1) - float64(downOpNum)/float64(downOpPara)) * float64(num) * caps.OpsMap[task].ResRequirement.Network
					}
				}
			}
			// Get max for plan cost
			if cpcost > planCost.Compute {
				planCost.Compute = cpcost
			}
			if iocost > planCost.State {
				planCost.State = iocost
			}
			if nwcost > planCost.Network {
				planCost.Network = nwcost
			}
		}
		// Round the planCost same way as the caps cost (keep 2 decimal digits)
		scaleFactor := math.Pow(10, 2)
		planCost.Compute = math.Round(planCost.Compute*scaleFactor) / scaleFactor
		planCost.State = math.Round(planCost.State*scaleFactor) / scaleFactor
		planCost.Network = math.Round(planCost.Network*scaleFactor) / scaleFactor
		// Test if the plan cost matches
		if !(planCost.Compute == plan.Cost.Compute && planCost.State == plan.Cost.State && planCost.Network == plan.Cost.Network) {
			fmt.Println("[Test Error] Plan cost calculation mismatch!")
			fmt.Println("			Local		Search")
			fmt.Println("Compute:	", planCost.Compute, plan.Cost.Compute)
			fmt.Println("State:		", planCost.State, plan.Cost.State)
			fmt.Println("Network:	", planCost.Network, plan.Cost.Network)
			os.Exit(1)
		}
	}
	fmt.Println("[Pass] Cost for all plans are correct!")
}
