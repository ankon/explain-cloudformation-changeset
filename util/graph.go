package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/goccy/go-graphviz/cgraph"
	log "github.com/sirupsen/logrus"
)

type changeSetGraph struct {
	rootGraph *cgraph.Graph
	// All graphs, indexed by StackName
	graphs map[string]*cgraph.Graph

	// Nodes, in a "flat" map indexed by StackName.LogicalResourceId
	nodes map[string]*cgraph.Node
}

type cloudformationClient interface {
	cloudformation.DescribeChangeSetAPIClient
}

type color = string

const (
	modifiedResourceColor color = "/paired10/2"
	addedResourceColor    color = "/paired10/4"
	removedResourceColor  color = "/paired10/6"
	importedResourceColor color = "/paired10/8"
	dynamicResourceColor  color = "/paired10/12"

	unusedParameterColor color = "/paired10/9"
	usedParameterColor   color = "/paired10/10"

	maybeReplacedResourceFillColor color = "/paired10/1"
	replacedResourceFillColor      color = "/paired10/2"
	removedResourceFillColor       color = "/paired10/5"

	parametersNodeName = "Parameters"
	stackNodeName      = "_"
)

func contains[E comparable](s []E, e E) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// TODO: Redesign the API, parameter nodes are very different than resource (change) nodes
type configureNodeFunc func(*cgraph.Node) error

func configureParameterNode(node *cgraph.Node) error {
	// Label will be build incrementally when changes are caused by parameters
	node.SetLabel("")
	node.SetShape("record")
	return nil
}

func configureResourceChangeNode(clusterName *string) configureNodeFunc {
	return func(node *cgraph.Node) error {
		node.SetShape(cgraph.BoxShape)
		if clusterName != nil {
			// XXX: We could JSON-encode things here if needed
			node.SetComment(*clusterName)
		}
		return nil
	}
}

func (csg *changeSetGraph) makeStack(parentStackName string, stackName string, name string) (*cgraph.Graph, error) {
	_, present := csg.graphs[stackName]
	if present {
		return nil, fmt.Errorf("graph for stack %q exists?", stackName)
	}

	// Don't know the graph yet, so let's build one as part of the parent
	parentGraph, present := csg.graphs[parentStackName]
	if !present {
		return nil, fmt.Errorf("cannot find graph for parent stack %q", parentStackName)
	}

	// "cluster_" prefix is needed to draw the box around the subgraph
	graph := parentGraph.SubGraph(fmt.Sprintf("cluster_%s", stackName), 1)
	graph.SetLabel(fmt.Sprintf("%s\n%s", name, stackName))
	csg.graphs[stackName] = graph
	return graph, nil
}

func (*changeSetGraph) makeNodeId(stackName string, name string) string {
	return fmt.Sprintf("%s.%s", stackName, name)
}

func (csg *changeSetGraph) findNode(stackName string, name string) (*cgraph.Node, error) {
	nodeId := csg.makeNodeId(stackName, name)
	node, present := csg.nodes[nodeId]
	if !present {
		return nil, fmt.Errorf("cannot find node %v", nodeId)
	}
	return node, nil
}

func (csg *changeSetGraph) makeOrFindNode(stackName, name string, configureNode configureNodeFunc) (*cgraph.Node, error) {
	nodeId := csg.makeNodeId(stackName, name)

	var node *cgraph.Node
	node, present := csg.nodes[nodeId]
	if !present {
		log.Infof("creating node %q", nodeId)

		graph, present := csg.graphs[stackName]
		if !present {
			return nil, fmt.Errorf("cannot find stack graph %s", stackName)
		}

		newNode, err := graph.CreateNode(nodeId)
		if err != nil {
			return nil, fmt.Errorf("cannot create node in graph, %v", err)
		}

		configureNode(newNode)

		node = newNode
		csg.nodes[nodeId] = newNode
	}

	return node, nil
}

type changeCause struct {
	node *cgraph.Node
	// If set: a port on this node to connect
	port *string

	// The detail information about this cause
	detail types.ResourceChangeDetail
}

func makeTargetHash(target *types.ResourceTargetDefinition) string {
	return fmt.Sprintf("%s.%s", target.Attribute, aws.ToString(target.Name))
}

func (csg *changeSetGraph) findChangeCauses(stackName string, change *types.ResourceChange) ([]changeCause, error) {
	result := []changeCause{}

	// Walk through all targets, and try to figure out what they refer to.
	// See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-cfn-updating-stacks-changesets-samples.html for
	// some good examples and hints on how to read the changes
	for _, detail := range change.Details {
		causingEntity := aws.ToString(detail.CausingEntity)

		if causingEntity == "" && detail.Evaluation == types.EvaluationTypeDynamic {
			// Changes to parameters produce both a "dynamic" and a "static" change for the same target. The "static"
			// change has better information, and we want to skip the dynamic one in that case.
			targetHash := makeTargetHash(detail.Target)
			foundMatchingStaticDetail := false
			for _, innerDetail := range change.Details {
				if innerDetail.Evaluation == types.EvaluationTypeStatic {
					innerTargetHash := makeTargetHash(innerDetail.Target)
					if innerTargetHash == targetHash {
						foundMatchingStaticDetail = true
						break
					}
				}
			}
			if foundMatchingStaticDetail {
				continue
			}
		}

		switch detail.ChangeSource {
		case types.ChangeSourceDirectModification:
			helperNode, err := csg.makeOrFindNode(stackName, "Direct modification", func(node *cgraph.Node) error {
				node.SetLabel("Direct modification")
				node.SetShape(cgraph.NoneShape)
				return nil
			})
			if err == nil {
				result = append(result, changeCause{helperNode, nil, detail})
			}
		case types.ChangeSourceParameterReference:
			node, err := csg.findNode(stackName, parametersNodeName)
			if err != nil {
				// Not yet there, just make it
				node, err = csg.makeOrFindNode(stackName, parametersNodeName, configureParameterNode)
			}
			if err == nil {
				// Check the label, we may need to add the parameter
				// We want the properties record to be always TB ranking, so flip the direction if needed
				// XXX: Ugly, do this with a property?
				var parseLabel func(string) []string
				var format string
				if csg.rootGraph.Get("rankdir") == "LR" || csg.rootGraph.Get("rankdir") == "RL" {
					format = "{%s}"
					parseLabel = func(s string) []string {
						return strings.Split(s[1:len(s)-1], "|")
					}
				} else {
					format = "%s"
					parseLabel = func(s string) []string {
						return strings.Split(s, "|")
					}
				}

				parameterSpec := fmt.Sprintf("<%s>%s", causingEntity, causingEntity)
				label := node.Get("label")
				if label == "" {
					node.SetLabel(fmt.Sprintf(format, parameterSpec))
				} else {
					knownParameterSpecs := parseLabel(label)
					if !contains(knownParameterSpecs, parameterSpec) {
						newLabel := fmt.Sprintf(format, strings.Join(append(knownParameterSpecs, parameterSpec), "|"))
						node.SetLabel(newLabel)
					}
				}

				node.SetColor(usedParameterColor)
				result = append(result, changeCause{node, &causingEntity, detail})
			}
		case types.ChangeSourceResourceReference:
			// XXX: We could "record" this, too?
			node, err := csg.makeOrFindNode(stackName, causingEntity, configureResourceChangeNode(nil))
			if err == nil {
				result = append(result, changeCause{node, nil, detail})
			}
		case types.ChangeSourceResourceAttribute:
			// CausingEntity is "LogicalResourceId.Attribute", for a nested stack "Attribute" could also be "Outputs.NameOfOutput"
			// We only care about the first part here
			// XXX: We could "record" this, too?
			logicalResourceId := causingEntity[:strings.IndexByte(causingEntity, '.')]
			node, err := csg.makeOrFindNode(stackName, logicalResourceId, configureResourceChangeNode(nil))
			if err == nil {
				result = append(result, changeCause{node, nil, detail})
			}
		}
	}

	if len(result) == 0 {
		log.Debugf("cannot find any understood change cause, %v", change)
	}
	return result, nil
}

type resourceNode interface {
	SetColors(border string, fill string)
	SetLabel(string)
}

type graphResourceNode struct {
	*cgraph.Graph
}

func (g *graphResourceNode) SetColors(border string, fill string) {
	g.Graph.SafeSet("color", border, "")
	g.Graph.SafeSet("fillcolor", fill, "")
	if fill != "" {
		g.Graph.SetStyle(cgraph.FilledGraphStyle)
	}
}
func (g *graphResourceNode) SetLabel(s string) {
	g.Graph.SetLabel(s)
}

type nodeResourceNode struct {
	*cgraph.Node
}

func (n *nodeResourceNode) SetColors(border string, fill string) {
	n.Node.SetColor(border)
	n.Node.SetFillColor(fill)
	if fill != "" {
		n.Node.SetStyle(cgraph.FilledNodeStyle)
	}
}
func (n *nodeResourceNode) SetLabel(s string) {
	n.Node.SetLabel(s)
}

func makeResourceNode(node interface{}) (resourceNode, error) {
	switch node := node.(type) {
	case *cgraph.Graph:
		return &graphResourceNode{node}, nil
	case *cgraph.Node:
		return &nodeResourceNode{node}, nil
	}

	return nil, fmt.Errorf("incompatible node type %T", node)
}

func configureResourceNode(node resourceNode, change types.ResourceChange, logicalResourceId string) {
	var fillColor string
	switch change.Replacement {
	case types.ReplacementTrue:
		fillColor = replacedResourceFillColor
	case types.ReplacementConditional:
		fillColor = maybeReplacedResourceFillColor
	}

	var changeTypePrefix string
	switch change.Action {
	case types.ChangeActionAdd:
		changeTypePrefix = "+"
		node.SetColors(addedResourceColor, fillColor)
	case types.ChangeActionRemove:
		changeTypePrefix = "-"
		node.SetColors(removedResourceColor, removedResourceFillColor)
	case types.ChangeActionModify:
		changeTypePrefix = "~"
		node.SetColors(modifiedResourceColor, fillColor)
	case types.ChangeActionImport:
		changeTypePrefix = "*"
		node.SetColors(importedResourceColor, fillColor)
	case types.ChangeActionDynamic:
		changeTypePrefix = "?"
		node.SetColors(dynamicResourceColor, fillColor)
	}
	node.SetLabel(fmt.Sprintf("%s %s\n%s", changeTypePrefix, logicalResourceId, aws.ToString(change.ResourceType)))
}

func (csg *changeSetGraph) populateGraph(svc cloudformationClient, resp *cloudformation.DescribeChangeSetOutput) error {
	// Nodes: 1. Resources that changed (name: StackName.LogicalResourceId)
	//        2. Provided parameter (name: StackName.ParameterKey)
	// Edges: 1. Cause-of-change
	//        2. Nested stack
	// nested stacks are subgraphs
	// Coloring: resource/parameter (used/unused), cause

	stackName := aws.ToString(resp.StackName)

	pass2Changes := []types.Change{}

	// Phase 1: Walk over the changes and build the nodes for all involved resources (as well as the sub-graphs for nested stacks)
	for _, change := range resp.Changes {
		logicalResourceId := aws.ToString(change.ResourceChange.LogicalResourceId)
		// If this change is a nested stack we make up a "fake" node as root of the stack where we point to, and
		// adjust the edges to point to the subgraph instead
		resourceType := aws.ToString(change.ResourceChange.ResourceType)
		isNestedStack := resourceType == "AWS::CloudFormation::Stack"

		var node resourceNode
		if isNestedStack {
			log.Infof("processing %q nested stack %v.%v", change.ResourceChange.Action, stackName, logicalResourceId)

			var nestedChangeSet *cloudformation.DescribeChangeSetOutput
			var nestedStackName string
			if change.ResourceChange.ChangeSetId != nil {
				// Query the change set of that stack, which will also reveal the actual stack name
				tmp, err := svc.DescribeChangeSet(context.TODO(), &cloudformation.DescribeChangeSetInput{
					ChangeSetName: change.ResourceChange.ChangeSetId,
				})
				if err != nil {
					return fmt.Errorf("failed to get changeset, %v", err)
				}

				// Prepare the graph for the stack
				nestedChangeSet = tmp
				nestedStackName = aws.ToString(nestedChangeSet.StackName)
			} else {
				// Parse the stack name out of the ARN
				arn, err := arn.Parse(aws.ToString(change.ResourceChange.PhysicalResourceId))
				if err != nil {
					// Odd?
					return fmt.Errorf("failed to parse physical resource id of nested stack as ARN, %v", err)
				}
				parts := strings.Split(arn.Resource, "/")
				nestedStackName = parts[1]
			}

			nestedGraph, err := csg.makeStack(stackName, nestedStackName, logicalResourceId)
			if err != nil {
				return fmt.Errorf("cannot make subgraph for nested stack change, %v", err)
			}

			// Populate the graph with everything going on inside that stack
			// XXX: We could look at the template here if there is no changeset?
			if nestedChangeSet != nil {
				csg.populateGraph(svc, nestedChangeSet)
			}

			clusterName := fmt.Sprintf("cluster_%s", nestedStackName)
			changedNode, err := csg.makeOrFindNode(nestedStackName, stackNodeName, configureResourceChangeNode(&clusterName))
			if err != nil {
				return fmt.Errorf("cannot make node for nested stack change, %v", err)
			}
			// Hide this node, we'll adjust the edges to point to the subgraph
			changedNode.SetShape(cgraph.NoneShape)
			changedNode.SetLabel("")
			changedNode.SetStyle("invis")

			// Make this node also available through the original resource name
			nodeName := csg.makeNodeId(stackName, logicalResourceId)
			existingNode, present := csg.nodes[nodeName]
			if present {
				// This should never happen. If it does: Try to hide the node, as we cannot remove a
				// node from a graph.
				log.Warnf("Found existing node %q, trying to hide it", nodeName)
				existingNode.SetStyle("invis")
			}
			csg.nodes[nodeName] = changedNode

			node, err = makeResourceNode(nestedGraph)
			if err != nil {
				return fmt.Errorf("cannot create resource node for subgraph, %v", err)
			}
		} else {
			var err error
			changedNode, err := csg.makeOrFindNode(stackName, logicalResourceId, configureResourceChangeNode(nil))
			if err != nil {
				return fmt.Errorf("cannot make node for change, %v", err)
			}
			node, err = makeResourceNode(changedNode)
			if err != nil {
				return fmt.Errorf("cannot create resource node for node, %v", err)
			}
		}

		if change.Type != types.ChangeTypeResource {
			// We cannot handle these, someone needs to actually update the code.
			node.SetLabel(fmt.Sprintf("%v", change.Type))
			continue
		}

		configureResourceNode(node, *change.ResourceChange, logicalResourceId)

		if len(change.ResourceChange.Details) > 0 {
			pass2Changes = append(pass2Changes, change)
		}
	}

	// Phase 2: Build edges between nodes and the cause of their change
	for _, change := range pass2Changes {
		changeCauseNodes, err := csg.findChangeCauses(stackName, change.ResourceChange)
		if err != nil {
			return fmt.Errorf("cannot find node for change cause (change: %v), %v", change, err)
		}

		logicalResourceId := aws.ToString(change.ResourceChange.LogicalResourceId)
		changedNode, err := csg.findNode(stackName, logicalResourceId)
		if err != nil {
			return fmt.Errorf("cannot find changed node %s.%s", stackName, logicalResourceId)
		}

		for _, changeCause := range changeCauseNodes {
			edgeName := string(changeCause.detail.ChangeSource)
			log.Infof("creating edge %q from %q to %q", edgeName, changeCause.node.Get("id"), changedNode.Get("id"))

			e, err := csg.graphs[stackName].CreateEdge(edgeName, changeCause.node, changedNode)
			if err != nil {
				return fmt.Errorf("cannot make edge, %v", err)
			}

			if changeCause.port != nil {
				e.SetTailPort(*changeCause.port)
			}

			switch changeCause.detail.Evaluation {
			case types.EvaluationTypeStatic:
				e.SetStyle(cgraph.SolidEdgeStyle)
				e.SetTooltip("Static evaluation")
			case types.EvaluationTypeDynamic:
				e.SetStyle(cgraph.DashedEdgeStyle)
				e.SetTooltip("Dynamic evaluation")
			}

			targetName := aws.ToString(changeCause.detail.Target.Name)
			// XXX: "Parameters" is pretty much the default for nested stacks, and just adds noise. But, is this check
			//      enough, or could this now catch and hide user-defined things called
			if changeCause.detail.Target.Attribute == types.ResourceAttributeProperties && targetName != "Parameters" {
				e.SetHeadLabel(targetName)
			} else if changeCause.detail.Target.Attribute != types.ResourceAttributeProperties {
				e.SetHeadLabel(string(changeCause.detail.Target.Attribute))
			}

			// Show the source attribute
			// XXX: This produces a mess because it might overlap with the port for where the cause of a change _to_ the thing
			//      gets rendered, and we really want to see that part.
			//      For nested stack outputs it might be helpful to build an Outputs record?
			// if changeCause.detail.ChangeSource == types.ChangeSourceResourceAttribute {
			// 	causingEntity := aws.ToString(changeCause.detail.CausingEntity)
			// 	attributeName := causingEntity[strings.IndexByte(causingEntity, '.')+1:]
			// 	e.SetTailLabel(attributeName)
			// } // TODO: others? Note that Parameters have their box already.

			causeClusterName := changeCause.node.Get("comment")
			if causeClusterName != "" {
				e.SetLogicalTail(causeClusterName)
			}
			changedClusterName := changedNode.Get("comment")
			if changedClusterName != "" {
				e.SetLogicalHead(changedClusterName)
			}
		}
	}

	return nil
}

func NewChangeSetGraph(graph *cgraph.Graph, svc cloudformationClient, stackName string, rootChangeSetName string) (*changeSetGraph, error) {
	// Build the request with its input parameters
	params := &cloudformation.DescribeChangeSetInput{
		ChangeSetName: aws.String(rootChangeSetName),
	}
	if stackName != "" {
		params.StackName = aws.String(stackName)
	}
	resp, err := svc.DescribeChangeSet(context.TODO(), params)
	if err != nil {
		return nil, fmt.Errorf("failed to get changeset, %v", err)
	}

	// We want subgraphs with logical heads
	graph.SetCompound(true)

	graphs := map[string]*cgraph.Graph{
		aws.ToString(resp.StackName): graph,
	}
	nodes := map[string]*cgraph.Node{}
	result := &changeSetGraph{graph, graphs, nodes}

	err = result.populateGraph(svc, resp)
	if err != nil {
		return nil, err
	}
	return result, nil
}
