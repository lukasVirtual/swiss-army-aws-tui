package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

// ResourcesTab represents the AWS resources tab
type ResourcesTab struct {
	// Core components
	view      *tview.Flex
	app       *tview.Application
	awsClient *aws.Client
	eventChan chan<- Event

	// UI components
	serviceList   *tview.List
	resourceTable *tview.Table
	resourceInfo  *tview.TextView
	statusText    *tview.TextView
	filterInput   *tview.InputField

	// State
	selectedService string
	resources       map[string][]Resource
	filteredRes     []Resource
	selectedRes     *Resource
	mu              sync.RWMutex
	loading         bool
}

// Resource represents an AWS resource
type Resource struct {
	ID          string
	Name        string
	Type        string
	State       string
	Region      string
	CreatedDate string
	Tags        map[string]string
	Details     map[string]interface{}
}

// ServiceInfo represents information about an AWS service
type ServiceInfo struct {
	Name        string
	DisplayName string
	Icon        string
	Enabled     bool
}

var supportedServices = []ServiceInfo{
	{Name: "ec2", DisplayName: "EC2 Instances", Icon: "🤖", Enabled: true},
	{Name: "s3", DisplayName: "S3 Buckets", Icon: "🪣", Enabled: true},
	{Name: "rds", DisplayName: "RDS Databases", Icon: "📚", Enabled: true},
	{Name: "lambda", DisplayName: "Lambda Functions", Icon: "⚡", Enabled: true},
	{Name: "ecs", DisplayName: "ECS Services", Icon: "🐳", Enabled: true},
	{Name: "vpc", DisplayName: "VPC Networks", Icon: "🌐", Enabled: true},
	{Name: "iam", DisplayName: "IAM Resources", Icon: "🔐", Enabled: false},
	{Name: "cloudformation", DisplayName: "CloudFormation", Icon: "📚", Enabled: false},
}

// NewResourcesTab creates a new resources tab
func NewResourcesTab(app *tview.Application, eventChan chan<- Event) (*ResourcesTab, error) {
	tab := &ResourcesTab{
		app:       app,
		eventChan: eventChan,
		resources: make(map[string][]Resource),
	}

	if err := tab.initializeUI(); err != nil {
		return nil, fmt.Errorf("failed to initialize resources tab UI: %w", err)
	}

	logger.Info("ResourcesTab initialized")
	return tab, nil
}

// initializeUI initializes the UI components
func (rt *ResourcesTab) initializeUI() error {
	// Create service list
	rt.serviceList = tview.NewList().
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorWhite).
		ShowSecondaryText(true)

	rt.serviceList.SetBorder(true).SetTitle(" AWS Services ").SetTitleAlign(tview.AlignLeft)

	// Set up service list handlers
	rt.serviceList.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		logger.Info("Service selected", zap.Int("index", index), zap.String("mainText", mainText))
		rt.onServiceSelected(index, mainText, secondaryText, shortcut)
	})
	rt.serviceList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		logger.Info("Service highlighted", zap.Int("index", index), zap.String("mainText", mainText))
		rt.onServiceHighlighted(index, mainText, secondaryText, shortcut)
	})

	// Add key bindings for service list
	rt.serviceList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			rt.Refresh()
			return nil
		case 'f':
			rt.focusFilter()
			return nil
		}
		return event
	})

	// Create filter input
	rt.filterInput = tview.NewInputField().
		SetLabel("Filter: ").
		SetFieldWidth(0).
		SetChangedFunc(rt.onFilterChanged)

	rt.filterInput.SetBorder(true).SetTitle(" Filter Resources ").SetTitleAlign(tview.AlignLeft)

	// Create resource table
	if rt.resourceTable != nil {
		rt.resourceTable.Clear() // Clear existing table to prevent duplication
	}
	rt.resourceTable = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack).Attributes(tcell.AttrBold))

	rt.resourceTable.SetBorder(true).SetTitle(" Resources ").SetTitleAlign(tview.AlignLeft)

	// Set up resource table handlers
	rt.resourceTable.SetSelectedFunc(rt.onResourceSelected)
	rt.resourceTable.SetSelectionChangedFunc(rt.onResourceHighlighted)

	// Add key bindings for resource table
	rt.resourceTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			rt.Refresh()
			return nil
		case 'f':
			rt.focusFilter()
			return nil
		case 'l':
			rt.onLambdaLogsKey()
			return nil
		case 's':
			rt.onEC2StartInstance()
			return nil
		case 'p':
			rt.onEC2StopInstance()
			return nil
		}
		logger.Info("Service list key pressed", zap.String("key", event.Name()))
		logger.Info("Resource table key pressed", zap.String("key", event.Name()))
		return event
	})

	// Create resource info panel
	rt.resourceInfo = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true)

	rt.resourceInfo.SetBorder(true).SetTitle(" Resource Details ").SetTitleAlign(tview.AlignLeft)

	// Create status text
	rt.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	rt.statusText.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)
	rt.updateStatus("No AWS client configured", "yellow")

	// Load services into list
	rt.loadServices()

	// Create layout
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(rt.serviceList, 0, 2, true).
		AddItem(rt.filterInput, 3, 0, false).
		AddItem(rt.statusText, 5, 0, false)

	centerPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(rt.resourceTable, 0, 1, false)

	rt.view = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 30, 0, true).
		AddItem(centerPanel, 0, 2, false).
		AddItem(rt.resourceInfo, 40, 0, false)

	return nil
}

// loadServices loads AWS services into the service list
func (rt *ResourcesTab) loadServices() {
	rt.serviceList.Clear()

	for i, service := range supportedServices {
		mainText := fmt.Sprintf("%s %s", service.Icon, service.DisplayName)
		// secondaryText := service.Name
		secondaryText := ""

		if !service.Enabled {
			mainText = fmt.Sprintf("[gray]%s (Coming Soon)[-]", mainText)
			secondaryText = "Not implemented yet"
		}

		rt.serviceList.AddItem(mainText, secondaryText, rune('0'+i%10), func() {
			if service.Enabled {
				rt.selectService(service.Name)
			}
		})
	}
}

// onServiceSelected handles service selection
func (rt *ResourcesTab) onServiceSelected(index int, mainText, secondaryText string, shortcut rune) {
	if index >= 0 && index < len(supportedServices) {
		service := supportedServices[index]
		if service.Enabled {
			rt.selectService(service.Name)
		}
	}
}

// onServiceHighlighted handles service highlighting
func (rt *ResourcesTab) onServiceHighlighted(index int, mainText, secondaryText string, shortcut rune) {
	if index >= 0 && index < len(supportedServices) {
		service := supportedServices[index]
		rt.updateResourceInfo(fmt.Sprintf("Service: %s\n\nSelect this service to view resources.", service.DisplayName))
	}
}

// selectService selects a service and loads its resources
func (rt *ResourcesTab) selectService(serviceName string) {
	if rt.awsClient == nil {
		rt.updateStatus("No AWS client configured", "yellow")
		return
	}

	rt.mu.Lock()
	rt.selectedService = serviceName
	rt.loading = true
	rt.mu.Unlock()

	logger.Info("Selecting service", zap.String("service", serviceName))
	rt.updateStatus("Loading resources...", "yellow")

	go rt.loadResourcesAsync(serviceName)
}

// loadResourcesAsync loads resources for a service asynchronously
func (rt *ResourcesTab) loadResourcesAsync(serviceName string) {
	defer func() {
		rt.mu.Lock()
		rt.loading = false
		rt.mu.Unlock()
	}()

	var resources []Resource
	var err error

	switch serviceName {
	case "ec2":
		resources, err = rt.loadEC2Instances()
	case "s3":
		resources, err = rt.loadS3Buckets()
	case "rds":
		resources, err = rt.loadRDSInstances()
	case "lambda":
		resources, err = rt.loadLambdaFunctions()
	case "ecs":
		resources, err = rt.loadECSServices()
	case "vpc":
		resources, err = rt.loadVPCs()
	default:
		err = fmt.Errorf("service %s not implemented", serviceName)
	}

	if err != nil {
		logger.Error("Failed to load resources", zap.String("service", serviceName), zap.Error(err))
		if rt.app != nil {
			rt.app.QueueUpdateDraw(func() {
				rt.updateStatus(fmt.Sprintf("Error loading %s: %s", serviceName, err.Error()), "red")
			})
		}
		return
	}

	rt.mu.Lock()
	rt.resources[serviceName] = resources
	rt.mu.Unlock()

	if rt.app != nil {
		rt.app.QueueUpdateDraw(func() {
			rt.updateResourceTable(resources)
			rt.updateStatus(fmt.Sprintf("Loaded %d %s resources", len(resources), serviceName), "green")
		})
	}

	logger.Info("Loaded resources", zap.String("service", serviceName), zap.Int("count", len(resources)))
}

// loadEC2Instances loads EC2 instances
func (rt *ResourcesTab) loadEC2Instances() ([]Resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	instances, err := rt.awsClient.GetEC2FunctionDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	var resources []Resource

	for _, instance := range instances {
		res := ec2InstanceToResource(instance, rt.awsClient.GetRegion())
		resources = append(resources, res)
	}

	return resources, nil
}

// loadS3Buckets loads S3 buckets
func (rt *ResourcesTab) loadS3Buckets() ([]Resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := rt.awsClient.GetS3FunctionDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	var resources []Resource

	for i, detail := range details {
		region := detail.Region
		if region == "" {
			region = rt.awsClient.GetRegion()
		}

		resource := Resource{
			ID:     strconv.Itoa(i),
			Name:   detail.Name,
			Type:   "S3 Bucket",
			State:  "Available",
			Region: region,
			Tags:   make(map[string]string),
		}

		if detail.CreationDate != nil {
			resource.CreatedDate = detail.CreationDate.Format("2006-01-02 15:04:05")
		}

		resource.Details = map[string]interface{}{
			"BucketName": detail.Name,
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

func ec2InstanceToResource(instance types.Instance, region string) Resource {
	res := Resource{
		Type:   "EC2 Instance",
		State:  string(instance.State.Name),
		Region: region,
		Tags:   make(map[string]string),
	}

	if instance.InstanceId != nil {
		res.ID = *instance.InstanceId
	}

	if instance.LaunchTime != nil {
		res.CreatedDate = instance.LaunchTime.Format("2006-01-02 15:04:05")
	}

	for _, tag := range instance.Tags {
		if tag.Key != nil && tag.Value != nil {
			res.Tags[*tag.Key] = *tag.Value
			if *tag.Key == "Name" {
				res.Name = *tag.Value
			}
		}
	}

	if res.Name == "" {
		res.Name = res.ID
	}

	res.Details = map[string]interface{}{
		"InstanceType":     string(instance.InstanceType),
		"ImageId":          getStringValue(instance.ImageId),
		"VpcId":            getStringValue(instance.VpcId),
		"SubnetId":         getStringValue(instance.SubnetId),
		"PublicIpAddress":  getStringValue(instance.PublicIpAddress),
		"PrivateIpAddress": getStringValue(instance.PrivateIpAddress),
		"KeyName":          getStringValue(instance.KeyName),
		"SecurityGroups":   instance.SecurityGroups,
	}

	return res
}

// loadRDSInstances loads RDS instances using the RDS service wrapper
func (rt *ResourcesTab) loadRDSInstances() ([]Resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := rt.awsClient.GetRDSFunctionDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to describe RDS instances: %w", err)
	}

	var resources []Resource
	for _, d := range details {
		createdDate := ""
		if d.InstanceCreateTime != nil {
			createdDate = d.InstanceCreateTime.Format("2006-01-02 15:04:05")
		}

		resource := Resource{
			ID:          d.DBInstanceIdentifier,
			Name:        d.DBInstanceIdentifier,
			Type:        "RDS Instance",
			State:       d.DBInstanceStatus,
			Region:      rt.awsClient.GetRegion(),
			CreatedDate: createdDate,
			Tags:        make(map[string]string),
			Details:     make(map[string]interface{}),
		}

		// Add additional details
		resource.Details["Engine"] = d.Engine
		resource.Details["Engine Version"] = d.EngineVersion
		resource.Details["Status"] = d.DBInstanceStatus
		resource.Details["Endpoint"] = d.Endpoint
		resource.Details["Allocated Storage (GB)"] = d.AllocatedStorage

		resources = append(resources, resource)
	}

	return resources, nil
}

// loadLambdaFunctions loads Lambda functions using the Lambda service wrapper.
func (rt *ResourcesTab) loadLambdaFunctions() ([]Resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use the higher-level lambda service wrapper on the aws client to get detailed metadata
	details, err := rt.awsClient.GetLambdaFunctionDetails(ctx)
	if err != nil {
		return nil, err
	}

	var resources []Resource
	for _, d := range details {
		res := Resource{
			ID:          d.FunctionName,
			Name:        d.FunctionName,
			Type:        "Lambda Function",
			State:       d.State,
			Region:      rt.awsClient.GetRegion(),
			CreatedDate: d.LastModified,
			Tags:        make(map[string]string),
			Details: map[string]interface{}{
				"Runtime":          d.Runtime,
				"Handler":          d.Handler,
				"MemorySize":       d.MemorySize,
				"Timeout":          d.Timeout,
				"Description":      d.Description,
				"CodeSize":         d.CodeSize,
				"SnapStartEnabled": d.SnapStartEnabled,
				"SnapStartStatus":  d.SnapStartStatus,
				"LogGroupName":     d.LogGroupName,
			},
		}
		resources = append(resources, res)
	}

	return resources, nil
}

// loadECSServices loads ECS services (placeholder)
func (rt *ResourcesTab) loadECSServices() ([]Resource, error) {
	// Placeholder implementation
	return []Resource{
		{
			ID:          "ecs-example-1",
			Name:        "Example ECS Service",
			Type:        "ECS Service",
			State:       "Running",
			Region:      rt.awsClient.GetRegion(),
			CreatedDate: time.Now().Format("2006-01-02 15:04:05"),
			Tags:        make(map[string]string),
			Details:     map[string]interface{}{"Note": "ECS implementation coming soon"},
		},
	}, nil
}

// loadVPCs loads VPCs (placeholder)
func (rt *ResourcesTab) loadVPCs() ([]Resource, error) {
	// Placeholder implementation
	return []Resource{
		{
			ID:          "vpc-example-1",
			Name:        "Example VPC",
			Type:        "VPC",
			State:       "Available",
			Region:      rt.awsClient.GetRegion(),
			CreatedDate: time.Now().Format("2006-01-02 15:04:05"),
			Tags:        make(map[string]string),
			Details:     map[string]interface{}{"Note": "VPC implementation coming soon"},
		},
	}, nil
}

// updateResourceTable updates the resource table with the given resources
func (rt *ResourcesTab) updateResourceTable(resources []Resource) {
	rt.filteredRes = resources
	rt.applyFilter()
}

// applyFilter applies the current filter to resources
func (rt *ResourcesTab) applyFilter() {
	filterText := strings.ToLower(strings.TrimSpace(rt.filterInput.GetText()))

	var filtered []Resource
	if filterText == "" {
		filtered = rt.filteredRes
	} else {
		for _, res := range rt.filteredRes {
			if strings.Contains(strings.ToLower(res.Name), filterText) ||
				strings.Contains(strings.ToLower(res.ID), filterText) ||
				strings.Contains(strings.ToLower(res.State), filterText) ||
				strings.Contains(strings.ToLower(res.Type), filterText) {
				filtered = append(filtered, res)
			}
		}
	}

	// Update table
	if rt.resourceTable != nil {
		logger.Info("Clearing resource table")
		rt.resourceTable.Clear()
	}

	// Add headers
	headers := []string{"Name", "ID", "Type", "State", "Region", "Created"}
	for col, header := range headers {
		rt.resourceTable.SetCell(0, col,
			tview.NewTableCell(header).
				SetTextColor(tcell.ColorYellow).
				SetAttributes(tcell.AttrBold))
	}

	// Add resources
	for row, resource := range filtered {
		rt.resourceTable.SetCell(row+1, 0, tview.NewTableCell(resource.Name))
		rt.resourceTable.SetCell(row+1, 1, tview.NewTableCell(resource.ID))
		rt.resourceTable.SetCell(row+1, 2, tview.NewTableCell(resource.Type))

		// Color-code state
		stateColor := tcell.ColorWhite
		switch strings.ToLower(resource.State) {
		case "running", "available", "active":
			stateColor = tcell.ColorGreen
		case "stopped", "terminated":
			stateColor = tcell.ColorRed
		case "pending", "stopping":
			stateColor = tcell.ColorYellow
		}
		rt.resourceTable.SetCell(row+1, 3,
			tview.NewTableCell(resource.State).SetTextColor(stateColor))

		rt.resourceTable.SetCell(row+1, 4, tview.NewTableCell(resource.Region))
		rt.resourceTable.SetCell(row+1, 5, tview.NewTableCell(resource.CreatedDate))
	}

	// Update title with count
	title := fmt.Sprintf(" Resources (%d", len(filtered))
	if len(filtered) != len(rt.filteredRes) {
		title += fmt.Sprintf(" of %d", len(rt.filteredRes))
	}
	title += ") "
	rt.resourceTable.SetTitle(title)
}

// onFilterChanged handles filter text changes
func (rt *ResourcesTab) onFilterChanged(text string) {
	rt.applyFilter()
}

// onResourceSelected handles resource selection
func (rt *ResourcesTab) onResourceSelected(row, column int) {
	if row <= 0 || row-1 >= len(rt.filteredRes) {
		return
	}

	resource := rt.filteredRes[row-1]
	rt.selectedRes = &resource
	rt.updateResourceDetails(&resource)
}

// onResourceHighlighted handles resource highlighting
func (rt *ResourcesTab) onResourceHighlighted(row, column int) {
	if row <= 0 || row-1 >= len(rt.filteredRes) {
		rt.updateResourceInfo("Select a resource to view details")
		return
	}

	resource := rt.filteredRes[row-1]
	rt.selectedRes = &resource
	rt.updateResourceDetails(&resource)
}

// updateResourceDetails updates the resource details panel
func (rt *ResourcesTab) updateResourceDetails(resource *Resource) {
	info := fmt.Sprintf(`[yellow]Name:[-] %s
[yellow]ID:[-] %s
[yellow]Type:[-] %s
[yellow]State:[-] %s
[yellow]Region:[-] %s
[yellow]Created:[-] %s

`, resource.Name, resource.ID, resource.Type, resource.State, resource.Region, resource.CreatedDate)

	// Add tags if any
	if len(resource.Tags) > 0 {
		info += "[yellow]Tags:[-]\n"
		var tagKeys []string
		for key := range resource.Tags {
			tagKeys = append(tagKeys, key)
		}
		sort.Strings(tagKeys)

		for _, key := range tagKeys {
			info += fmt.Sprintf("  %s: %s\n", key, resource.Tags[key])
		}
		info += "\n"
	}

	// Add details if any
	if len(resource.Details) > 0 {
		info += "[yellow]Details:[-]\n"
		var detailKeys []string
		for key := range resource.Details {
			detailKeys = append(detailKeys, key)
		}
		sort.Strings(detailKeys)

		for _, key := range detailKeys {
			info += fmt.Sprintf("  %s: %v\n", key, resource.Details[key])
		}
	}

	rt.updateResourceInfo(info)
}

// updateResourceInfo updates the resource info panel
func (rt *ResourcesTab) updateResourceInfo(text string) {
	// Guard against nil resourceInfo during initialization
	if rt.resourceInfo == nil {
		return
	}
	rt.resourceInfo.Clear() // Clear existing info to prevent duplication
	rt.resourceInfo.SetText(text)
}

// focusFilter focuses on the filter input field
func (rt *ResourcesTab) focusFilter() {
	// This would be called from the application level
}

// updateStatus updates the status display
func (rt *ResourcesTab) updateStatus(message, color string) {
	// Guard against nil statusText during initialization
	if rt.statusText == nil {
		return
	}
	rt.statusText.Clear() // Clear existing status to prevent duplication
	timestamp := time.Now().Format("15:04:05")
	statusText := fmt.Sprintf("[%s]%s[-]\n[gray]%s[-]", color, message, timestamp)
	rt.statusText.SetText(statusText)
}

// SetAWSClient sets the AWS client
func (rt *ResourcesTab) SetAWSClient(client *aws.Client) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.awsClient = client
	if client != nil {
		rt.updateStatus("AWS client configured", "green")
	} else {
		rt.updateStatus("AWS client removed", "yellow")
	}

	// Clear current resources
	rt.resources = make(map[string][]Resource)
	if rt.resourceTable != nil {
		logger.Info("Clearing resource table in SetAWSClient")
		rt.resourceTable.Clear()
	}
	rt.updateResourceInfo("Select a service to view resources")
}

// Refresh refreshes the current service resources
func (rt *ResourcesTab) Refresh() {
	rt.mu.RLock()
	service := rt.selectedService
	loading := rt.loading
	rt.mu.RUnlock()

	if loading {
		rt.updateStatus("Already loading...", "yellow")
		return
	}

	if service == "" {
		rt.updateStatus("No service selected", "yellow")
		return
	}

	if rt.resourceTable != nil {
		logger.Info("Clearing resource table in Refresh")
		rt.resourceTable.Clear() // Clear existing resources to prevent duplication
	}
	rt.selectService(service)
}

// GetView returns the main view component
func (rt *ResourcesTab) GetView() tview.Primitive {

	return rt.view
}
func (rt *ResourcesTab) onEC2StartInstance() {
	logger.Info("onEC2StartInstance called", zap.String("selectedService", rt.selectedService))

	if rt.selectedService != "ec2" {
		logger.Info("Not EC2 service, ignoring")
		return
	}

	if rt.selectedRes == nil {
		logger.Info("No resource selected")
		return
	}

	instanceID := rt.selectedRes.ID
	if instanceID == "" {
		rt.updateStatus("No InstanceId found for selected resource", "red")
		logger.Error("No InstanceId found in selected resource")
		return
	}

	rt.updateStatus(fmt.Sprintf("Starting EC2 instance %s...", instanceID), "yellow")

	// Since this is a UI-triggered asynchronous operation meant not to block the UI,
	// we do NOT generally use a WaitGroup for the user-facing routine.
	// State is protected with locks where appropriate, and UI updates are handled on the main UI goroutine.
	// If you ever need to clean up or synchronize these routines (such as cancelling/retrying),
	// consider keeping a list/context for outstanding operations, not just WaitGroups.

	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := rt.awsClient.GetClients().EC2.StartInstance(ctx, id)
		if err != nil {
			logger.Error("Failed to start EC2 instance", zap.String("instanceID", id), zap.Error(err))
			if rt.app != nil {
				rt.app.QueueUpdateDraw(func() {
					rt.updateStatus(fmt.Sprintf("Failed to start instance: %s", err.Error()), "red")
				})
			}
			return
		}

		logger.Info("EC2 instance started", zap.String("instanceID", id))
		if rt.app != nil {
			rt.app.QueueUpdateDraw(func() {
				rt.updateStatus(fmt.Sprintf("Instance %s started", id), "green")
				rt.Refresh()
			})
		}
	}(instanceID)
}

func (rt *ResourcesTab) onEC2StopInstance() {
	logger.Info("onEC2StopInstance called", zap.String("selectedService", rt.selectedService))

	if rt.selectedService != "ec2" {
		logger.Info("Not EC2 service, ignoring")
		return
	}

	if rt.selectedRes == nil {
		logger.Info("No resource selected")
		return
	}

	instanceID := rt.selectedRes.ID
	if instanceID == "" {
		rt.updateStatus("No InstanceId found for selected resource", "red")
		logger.Error("No InstanceId found in selected resource")
		return
	}

	rt.updateStatus(fmt.Sprintf("Stopping EC2 instance %s...", instanceID), "yellow")

	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := rt.awsClient.GetClients().EC2.StopInstance(ctx, id)
		if err != nil {
			logger.Error("Failed to stop EC2 instance", zap.String("instanceID", id), zap.Error(err))
			if rt.app != nil {
				rt.app.QueueUpdateDraw(func() {
					rt.updateStatus(fmt.Sprintf("Failed to stop instance: %s", err.Error()), "red")
				})
			}
			return
		}

		logger.Info("EC2 instance stopped", zap.String("instanceID", id))
		if rt.app != nil {
			rt.app.QueueUpdateDraw(func() {
				rt.updateStatus(fmt.Sprintf("Instance %s stopped", id), "green")
				rt.Refresh()
			})
		}
	}(instanceID)
}

func (rt *ResourcesTab) onLambdaLogsKey() {
	logger.Info("onLambdaLogsKey called", zap.String("selectedService", rt.selectedService))
	if rt.selectedService != "lambda" {
		logger.Info("Not lambda service, ignoring")
		return
	}

	if rt.selectedRes == nil {
		logger.Info("No resource selected")
		return
	}

	logGroup := ""
	if v, ok := rt.selectedRes.Details["LogGroupName"]; ok {
		if s, ok := v.(string); ok {
			logGroup = s
		}
	}

	if logGroup == "" {
		logGroup = fmt.Sprintf("/aws/lambda/%s", rt.selectedRes.Name)
	}

	logger.Info("Emitting EventShowLambdaLogs", zap.String("function", rt.selectedRes.Name), zap.String("logGroup", logGroup))
	if rt.eventChan != nil {
		data := map[string]string{
			"function": rt.selectedRes.Name,
			"logGroup": logGroup,
		}
		rt.eventChan <- Event{Type: EventShowLambdaLogs, Data: data}
	}
}

// getStringValue safely gets a string value from a pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
