package cloudformation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/awslabs/amazon-apigateway-ingress-controller/pkg/network"
	cfn "github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/apigateway"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"
	"github.com/awslabs/goformation/v4/cloudformation/route53"
	"github.com/awslabs/goformation/v4/cloudformation/tags"
	"github.com/awslabs/goformation/v4/cloudformation/wafv2"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
)

//const is constance values for resource naming used to build cf templates
const (
	AWSStackName                            = "AWS::StackName"
	AWSRegion                               = "AWS::Region"
	APIMethodResourceID                     = "Method"
	APIKeyResourceName                      = "APIKey"
	APIKeyUsagePlanResourceName             = "APIKeyUsagePlan"
	APIRootResourceResourceID               = "RootResourceId"
	APIResourceResourceName                 = "Resource"
	APIResourceName                         = "RestAPI"
	CustomDomainResourceName                = "CustomDomain"
	CustomDomainBasePathMappingResourceName = "CustomDomainBasePathMapping"
	DeploymentResourceName                  = "Deployment"
	DistributionDomainNameResourceName      = "DistributionDomainName"
	DistributionHostedZoneIdResourceName    = "DistributionHostedZoneId"
	LoadBalancerResourceName                = "LoadBalancer"
	ListnerResourceName                     = "Listener"
	RegionalDomainNameResourceName          = "RegionalDomainName"
	RegionalHostedZoneIdResourceName        = "RegionalHostedZoneId"
	SecurityGroupIngressResourceName        = "SecurityGroupIngress"
	TargetGroupResourceName                 = "TargetGroup"
	UsagePlanResourceName                   = "UsagePlan"
	VPCLinkResourceName                     = "VPCLink"
	WAFACLResourceName                      = "WAFAcl"
	WAFAssociationResourceName              = "WAFAssociation"
	Route53RecordResourceName               = "Route53RecordSet"
	OutputKeyRestAPIID                      = "RestAPIID"
	OutputKeyAPIGatewayEndpoint             = "APIGatewayEndpoint"
	OutputKeyAPIEndpointType                = "APIGWEndpointType"
	OutputKeyClientARNS                     = "ClientARNS"
	OutputKeyCertARN                        = "SSLCertArn"
	OutputKeyCustomDomain                   = "CustomDomainName"
	OutputKeyCustomDomainBasePath           = "CustomDomainBasePath"
	OutputMinimumCompressionSize            = "MinimumCompressionSize"
	OutputKeyWAFEnabled                     = "WAFEnabled"
	OutputKeyWAFRules                       = "WAFRules"
	OutputKeyWAFScope                       = "WAFScope"
	OutputKeyWAFAssociationCreated          = "WAFAssociation"
	OutputKeyCustomDomainHostName           = "CustomDomainHostname"
	OutputKeyCustomDomainHostedZoneID       = "CustomDomainHostedZoneID"
	OutputKeyHostedZone                     = "HostedZone"
	OutputKeyRequestTimeout                 = "RequestTimeout"
	OutputKeyTLSPolicy                      = "TLSPolicy"
	OutputKeyUsagePlans                     = "UsagePlansData"
)

func toLogicalName(idx int, parts []string) string {
	s := strings.Join(parts[:idx+1], "")
	remove := []string{"{", "}", "+", "-", "*"}
	for _, char := range remove {
		s = strings.Replace(s, char, "", -1)
	}
	return s
}

func toPath(idx int, parts []string) string {
	if parts[idx] == "{proxy+}" {
		return strings.Join(parts[:idx], "/") + "/{proxy}"
	}
	return strings.Join(parts[:idx+1], "/")
}

func mapApiGatewayMethodsAndResourcesFromPaths(paths []extensionsv1beta1.HTTPIngressPath, requestTimeout int, authorizationType string) map[string]cfn.Resource {
	m := map[string]cfn.Resource{}

	for _, path := range paths {
		parts := strings.Split(path.Path, "/")
		parts = append(parts, "{proxy+}")
		for idx, part := range parts {
			if idx == 0 {
				continue
			}
			ref := cfn.GetAtt(APIResourceName, APIRootResourceResourceID)
			if idx > 1 {
				ref = cfn.Ref(fmt.Sprintf("%s%s", APIResourceResourceName, toLogicalName(idx-1, parts)))
			}

			resourceLogicalName := fmt.Sprintf("%s%s", APIResourceResourceName, toLogicalName(idx, parts))
			m[resourceLogicalName] = buildAWSApiGatewayResource(ref, part)
			m[fmt.Sprintf("%s%s", APIMethodResourceID, toLogicalName(idx, parts))] = buildAWSApiGatewayMethod(resourceLogicalName, toPath(idx, parts), requestTimeout, authorizationType)
		}
	}

	return m
}

func buildAWSApiGatewayResource(ref, part string) *apigateway.Resource {
	return &apigateway.Resource{
		ParentId:  ref,
		PathPart:  part,
		RestApiId: cfn.Ref(APIResourceName),
	}
}

func buildAWSApiGatewayRestAPI(arns []string, apiEPType string, authorizationType string, minimumCompressionSize int) *apigateway.RestApi {
	if authorizationType == "AWS_IAM" && minimumCompressionSize > 0 {
		return &apigateway.RestApi{
			MinimumCompressionSize: minimumCompressionSize,
			ApiKeySourceType:       "HEADER",
			EndpointConfiguration: &apigateway.RestApi_EndpointConfiguration{
				Types: []string{apiEPType},
			},
			Name: cfn.Ref(AWSStackName),
			Policy: &PolicyDocument{
				Version: "2012-10-17",
				Statement: []Statement{
					{
						Action:    []string{"execute-api:Invoke"},
						Effect:    "Allow",
						Principal: map[string][]string{"AWS": arns},
						Resource:  []string{"*"},
					},
				},
			},
		}
	} else if authorizationType == "AWS_IAM" && minimumCompressionSize == 0 {
		return &apigateway.RestApi{
			ApiKeySourceType: "HEADER",
			EndpointConfiguration: &apigateway.RestApi_EndpointConfiguration{
				Types: []string{apiEPType},
			},
			Name: cfn.Ref(AWSStackName),
			Policy: &PolicyDocument{
				Version: "2012-10-17",
				Statement: []Statement{
					{
						Action:    []string{"execute-api:Invoke"},
						Effect:    "Allow",
						Principal: map[string][]string{"AWS": arns},
						Resource:  []string{"*"},
					},
				},
			},
		}
	} else if minimumCompressionSize > 0 {
		return &apigateway.RestApi{
			MinimumCompressionSize: minimumCompressionSize,
			ApiKeySourceType:       "HEADER",
			EndpointConfiguration: &apigateway.RestApi_EndpointConfiguration{
				Types: []string{apiEPType},
			},
			Name: cfn.Ref(AWSStackName),
			Policy: &AllPrinciplesPolicyDocument{
				Version: "2012-10-17",
				Statement: []AllPrinciplesStatement{
					{
						Action:    []string{"execute-api:Invoke"},
						Effect:    "Allow",
						Principal: "*",
						Resource:  []string{"*"},
					},
				},
			},
		}
	} else {
		return &apigateway.RestApi{
			ApiKeySourceType: "HEADER",
			EndpointConfiguration: &apigateway.RestApi_EndpointConfiguration{
				Types: []string{apiEPType},
			},
			Name: cfn.Ref(AWSStackName),
			Policy: &AllPrinciplesPolicyDocument{
				Version: "2012-10-17",
				Statement: []AllPrinciplesStatement{
					{
						Action:    []string{"execute-api:Invoke"},
						Effect:    "Allow",
						Principal: "*",
						Resource:  []string{"*"},
					},
				},
			},
		}
	}
}

type EmptyAction struct{}

func buildAWSWAFWebACL(webACLScope string, rules string) *wafv2.WebACL {
	waf := &wafv2.WebACL{
		Name:        cfn.Ref(AWSStackName),
		Scope:       webACLScope,
		Description: "This is an example WebACL",
		DefaultAction: &wafv2.WebACL_DefaultAction{
			Allow: EmptyAction{},
		},
		VisibilityConfig: &wafv2.WebACL_VisibilityConfig{
			SampledRequestsEnabled:   true,
			CloudWatchMetricsEnabled: true,
			MetricName:               cfn.Sub(fmt.Sprintf("${%s}WebACLMetric", AWSStackName)),
		},
	}

	if rules == "" {
		return waf
	}
	var wafRules []wafv2.WebACL_Rule
	if err := json.Unmarshal([]byte(rules), &wafRules); err != nil {
		return waf
	}
	waf.Rules = wafRules

	return waf
}

func buildAWSWAFWebACLAssociation(stage string) *wafv2.WebACLAssociation {
	wafAssociation := &wafv2.WebACLAssociation{
		WebACLArn:   cfn.GetAtt(WAFACLResourceName, "Arn"),
		ResourceArn: cfn.Sub(fmt.Sprintf("arn:aws:apigateway:${%s}::/restapis/${%s}/stages/%s", AWSRegion, APIResourceName, stage)),
	}

	dependsOn := []string{DeploymentResourceName, WAFACLResourceName}
	sort.Strings(dependsOn)
	wafAssociation.AWSCloudFormationDependsOn = dependsOn

	return wafAssociation
}

func buildAWSApiGatewayDeployment(stageName string, dependsOn []string) *apigateway.Deployment {
	d := &apigateway.Deployment{
		RestApiId: cfn.Ref(APIResourceName),
		StageName: stageName,
	}

	// Since we construct a map of in `mapApiGatewayMethodsAndResourcesFromPaths` we can't determine the order
	// that this list will be in - making it difficult to test - the order isn't important - but passing tests are.
	// This isn't the worst thing in the world - and - I considered refactoring - but I like how simple this is for now.
	// Also the order doesn't matter to CFN in the end.
	sort.Strings(dependsOn)
	d.AWSCloudFormationDependsOn = dependsOn

	return d
}

func buildAWSElasticLoadBalancingV2Listener() *elasticloadbalancingv2.Listener {
	return &elasticloadbalancingv2.Listener{
		LoadBalancerArn: cfn.Ref(LoadBalancerResourceName),
		Protocol:        "TCP",
		Port:            80,
		DefaultActions: []elasticloadbalancingv2.Listener_Action{
			elasticloadbalancingv2.Listener_Action{
				TargetGroupArn: cfn.Ref(TargetGroupResourceName),
				Type:           "forward",
			},
		},
	}
}

func buildAWSElasticLoadBalancingV2LoadBalancer(subnetIDs []string) *elasticloadbalancingv2.LoadBalancer {
	return &elasticloadbalancingv2.LoadBalancer{
		IpAddressType: "ipv4",
		Scheme:        "internal",
		Subnets:       subnetIDs,
		Tags: []tags.Tag{
			{
				Key:   "com.github.amazon-apigateway-ingress-controller/stack",
				Value: cfn.Ref(AWSStackName),
			},
		},
		Type: "network",
	}
}

func buildAWSElasticLoadBalancingV2TargetGroup(vpcID string, instanceIDs []string, nodePort int, dependsOn []string) *elasticloadbalancingv2.TargetGroup {
	targets := make([]elasticloadbalancingv2.TargetGroup_TargetDescription, len(instanceIDs))
	for i, instanceID := range instanceIDs {
		targets[i] = elasticloadbalancingv2.TargetGroup_TargetDescription{Id: instanceID}
	}

	return &elasticloadbalancingv2.TargetGroup{
		HealthCheckIntervalSeconds: 30,
		HealthCheckPort:            "traffic-port",
		HealthCheckProtocol:        "TCP",
		HealthCheckTimeoutSeconds:  10,
		HealthyThresholdCount:      3,
		Port:                       nodePort,
		Protocol:                   "TCP",
		Tags: []tags.Tag{
			{
				Key:   "com.github.amazon-apigateway-ingress-controller/stack",
				Value: cfn.Ref(AWSStackName),
			},
		},
		TargetType:              "instance",
		Targets:                 targets,
		UnhealthyThresholdCount: 3,
		VpcId:                   vpcID,
	}

}

func buildAWSApiGatewayVpcLink(dependsOn []string) *apigateway.VpcLink {
	r := &apigateway.VpcLink{
		Name:       cfn.Ref(AWSStackName),
		TargetArns: []string{cfn.Ref(LoadBalancerResourceName)},
	}

	r.AWSCloudFormationDependsOn = dependsOn

	return r
}

func buildAWSApiGatewayMethod(resourceLogicalName, path string, timeout int, authorizationType string) *apigateway.Method {
	var apiKeyRequired bool

	if authorizationType == "AWS_IAM" {
		apiKeyRequired = false
	} else {
		apiKeyRequired = true
	}

	m := &apigateway.Method{
		RequestParameters: map[string]bool{
			"method.request.path.proxy": true,
		},
		AuthorizationType: authorizationType,
		ApiKeyRequired:    apiKeyRequired,
		HttpMethod:        "ANY",
		ResourceId:        cfn.Ref(resourceLogicalName),
		RestApiId:         cfn.Ref(APIResourceName),
		Integration: &apigateway.Method_Integration{
			ConnectionId:          cfn.Ref(VPCLinkResourceName),
			ConnectionType:        "VPC_LINK",
			IntegrationHttpMethod: "ANY",
			PassthroughBehavior:   "WHEN_NO_MATCH",
			RequestParameters: map[string]string{
				"integration.request.path.proxy":             "method.request.path.proxy",
				"integration.request.header.Accept-Encoding": "'identity'",
			},
			Type:            "HTTP_PROXY",
			TimeoutInMillis: timeout,
			Uri:             cfn.Join("", []string{"http://", cfn.GetAtt(LoadBalancerResourceName, "DNSName"), path}),
		},
	}

	m.AWSCloudFormationDependsOn = []string{LoadBalancerResourceName}
	return m
}

func buildAWSEC2SecurityGroupIngresses(securityGroupIds []string, cidr string, nodePort int) []*ec2.SecurityGroupIngress {
	sgIngresses := make([]*ec2.SecurityGroupIngress, len(securityGroupIds))
	for i, sgID := range securityGroupIds {
		sgIngresses[i] = &ec2.SecurityGroupIngress{
			IpProtocol: "TCP",
			CidrIp:     cidr,
			FromPort:   nodePort,
			ToPort:     nodePort,
			GroupId:    sgID,
		}
	}

	return sgIngresses
}

func buildCustomDomainBasePathMapping(domainName string, stageName string, basePath string) *apigateway.BasePathMapping {
	var r *apigateway.BasePathMapping
	if basePath == "" {
		r = &apigateway.BasePathMapping{
			DomainName: domainName,
			RestApiId:  cfn.Ref(APIResourceName),
			Stage:      stageName,
		}
	} else {
		r = &apigateway.BasePathMapping{
			DomainName: domainName,
			RestApiId:  cfn.Ref(APIResourceName),
			Stage:      stageName,
			BasePath:   basePath,
		}
	}

	r.AWSCloudFormationDependsOn = []string{DeploymentResourceName}
	return r
}

func buildCustomDomain(domainName string, certificateArn string, apiEPType string, secPolicy string) *apigateway.DomainName {
	if apiEPType == "REGIONAL" {
		return &apigateway.DomainName{
			RegionalCertificateArn: certificateArn,
			DomainName:             domainName,
			SecurityPolicy:         secPolicy,
			EndpointConfiguration: &apigateway.DomainName_EndpointConfiguration{
				Types: []string{apiEPType},
			},
		}
	}
	return &apigateway.DomainName{
		CertificateArn: certificateArn,
		DomainName:     domainName,
		EndpointConfiguration: &apigateway.DomainName_EndpointConfiguration{
			Types: []string{apiEPType},
		},
	}

}

func buildUsagePlanAPIKeyMapping(usagePlan UsagePlan, i int) []*apigateway.UsagePlanKey {
	if usagePlan.APIKeys == nil {
		return nil
	}
	arr := make([]*apigateway.UsagePlanKey, len(usagePlan.APIKeys))
	for k, _ := range usagePlan.APIKeys {
		arr[k] = &apigateway.UsagePlanKey{
			KeyId:       cfn.Ref(fmt.Sprintf("%s%d%d", APIKeyResourceName, i, k)),
			KeyType:     "API_KEY",
			UsagePlanId: cfn.Ref(fmt.Sprintf("%s%d", UsagePlanResourceName, i)),
		}
	}
	return arr
}

func buildAPIKey(usagePlan UsagePlan) []*apigateway.ApiKey {
	if usagePlan.APIKeys == nil {
		return nil
	}
	arr := make([]*apigateway.ApiKey, len(usagePlan.APIKeys))
	for k, key := range usagePlan.APIKeys {
		arr[k] = &apigateway.ApiKey{
			CustomerId:         key.CustomerID,
			GenerateDistinctId: key.GenerateDistinctID,
			Name:               key.Name,
			Enabled:            true,
		}
	}
	return arr
}

func buildUsagePlan(usagePlan UsagePlan, stage string) *apigateway.UsagePlan {
	r := &apigateway.UsagePlan{
		UsagePlanName: usagePlan.PlanName,
		Description:   usagePlan.Description,
		Quota: &apigateway.UsagePlan_QuotaSettings{
			Limit:  usagePlan.QuotaLimit,
			Offset: usagePlan.QuotaOffset,
			Period: usagePlan.QuotaPeriod,
		},
		Throttle: &apigateway.UsagePlan_ThrottleSettings{
			BurstLimit: usagePlan.ThrottleBurstLimit,
			RateLimit:  usagePlan.ThrottleRateLimit,
		},
	}

	r.ApiStages = buildMethodThrottling(usagePlan.MethodThrottlingParameters, stage)

	r.AWSCloudFormationDependsOn = []string{DeploymentResourceName}

	return r
}

func buildMethodThrottling(methodThrottlingParameters []MethodThrottlingParametersObject, stage string) []apigateway.UsagePlan_ApiStage {
	rmap := make(map[string]apigateway.UsagePlan_ThrottleSettings)
	if methodThrottlingParameters != nil && len(methodThrottlingParameters) > 0 {
		for _, methodThrottlingParameter := range methodThrottlingParameters {
			r := apigateway.UsagePlan_ThrottleSettings{
				BurstLimit: methodThrottlingParameter.BurstLimit,
				RateLimit:  methodThrottlingParameter.RateLimit,
			}
			var key string
			if strings.HasSuffix(methodThrottlingParameter.Path, "/") {
				key = fmt.Sprintf("%sANY", methodThrottlingParameter.Path)
			} else {
				key = fmt.Sprintf("%s/ANY", methodThrottlingParameter.Path)
			}
			rmap[key] = r
		}
	}

	stageResource := apigateway.UsagePlan_ApiStage{
		ApiId:    cfn.Ref(APIResourceName),
		Stage:    stage,
		Throttle: rmap,
	}

	stageResources := []apigateway.UsagePlan_ApiStage{stageResource}

	return stageResources
}

//TemplateConfig is the structure of configuration used to provide data to build the cf template
type TemplateConfig struct {
	Network                *network.Network
	Rule                   extensionsv1beta1.IngressRule
	NodePort               int
	StageName              string
	Arns                   []string
	CustomDomainName       string
	CustomDomainBasePath   string
	CertificateArn         string
	APIEndpointType        string
	WAFEnabled             bool
	WAFRulesJSON           string
	WAFScope               string
	WAFAssociation         bool
	RequestTimeout         int
	TLSPolicy              string
	UsagePlans             []UsagePlan
	MinimumCompressionSize int
}

// BuildAPIGatewayTemplateFromIngressRule generates the cloudformation template according to the config provided
func BuildAPIGatewayTemplateFromIngressRule(cfg *TemplateConfig) *cfn.Template {
	template := cfn.NewTemplate()
	paths := cfg.Rule.IngressRuleValue.HTTP.Paths

	//Making default type edge
	if cfg.APIEndpointType == "" {
		cfg.APIEndpointType = "EDGE"
	}

	//Making default regional as cloudfront is not supported in all regions
	if cfg.WAFEnabled && cfg.WAFScope == "" {
		cfg.WAFScope = "REGIONAL"
	}

	var authorizationType string
	if cfg.UsagePlans != nil && len(cfg.UsagePlans) > 0 {
		authorizationType = "NONE"
	} else {
		authorizationType = "AWS_IAM"
	}

	methodLogicalNames := []string{}
	resourceMap := mapApiGatewayMethodsAndResourcesFromPaths(paths, cfg.RequestTimeout, authorizationType)
	for k, resource := range resourceMap {
		if _, ok := resource.(*apigateway.Method); ok {
			methodLogicalNames = append(methodLogicalNames, k)
		}
		template.Resources[k] = resource
	}

	targetGroup := buildAWSElasticLoadBalancingV2TargetGroup(*cfg.Network.Vpc.VpcId, cfg.Network.InstanceIDs, cfg.NodePort, []string{LoadBalancerResourceName})
	template.Resources[TargetGroupResourceName] = targetGroup

	listener := buildAWSElasticLoadBalancingV2Listener()
	template.Resources[ListnerResourceName] = listener

	securityGroupIngresses := buildAWSEC2SecurityGroupIngresses(cfg.Network.SecurityGroupIDs, *cfg.Network.Vpc.CidrBlock, cfg.NodePort)
	for i, sgI := range securityGroupIngresses {
		template.Resources[fmt.Sprintf("%s%d", SecurityGroupIngressResourceName, i)] = sgI
	}

	restAPI := buildAWSApiGatewayRestAPI(cfg.Arns, cfg.APIEndpointType, authorizationType, cfg.MinimumCompressionSize)
	template.Resources[APIResourceName] = restAPI

	deployment := buildAWSApiGatewayDeployment(cfg.StageName, methodLogicalNames)
	template.Resources[DeploymentResourceName] = deployment

	loadBalancer := buildAWSElasticLoadBalancingV2LoadBalancer(cfg.Network.SubnetIDs)
	template.Resources[LoadBalancerResourceName] = loadBalancer

	vPCLink := buildAWSApiGatewayVpcLink([]string{LoadBalancerResourceName})
	template.Resources[VPCLinkResourceName] = vPCLink

	if cfg.CustomDomainName != "" && cfg.CertificateArn != "" {
		customDomain := buildCustomDomain(cfg.CustomDomainName, cfg.CertificateArn, cfg.APIEndpointType, cfg.TLSPolicy)
		template.Resources[CustomDomainResourceName] = customDomain
		basePathMapping := buildCustomDomainBasePathMapping(cfg.CustomDomainName, cfg.StageName, cfg.CustomDomainBasePath)
		template.Resources[CustomDomainBasePathMappingResourceName] = basePathMapping
	}

	if cfg.UsagePlans != nil && len(cfg.UsagePlans) > 0 {
		for i, usagePlan := range cfg.UsagePlans {
			keyArr := buildAPIKey(usagePlan)
			for k, key := range keyArr {
				template.Resources[fmt.Sprintf("%s%d%d", APIKeyResourceName, i, k)] = key
			}
			template.Resources[fmt.Sprintf("%s%d", UsagePlanResourceName, i)] = buildUsagePlan(usagePlan, cfg.StageName)
			mapArr := buildUsagePlanAPIKeyMapping(usagePlan, i)
			for k, key := range mapArr {
				template.Resources[fmt.Sprintf("%s%d%d", APIKeyUsagePlanResourceName, i, k)] = key
			}
		}
	}

	if cfg.WAFEnabled {
		webACL := buildAWSWAFWebACL(cfg.WAFScope, cfg.WAFRulesJSON)
		template.Resources[WAFACLResourceName] = webACL
		if cfg.WAFAssociation {
			webACLAssociation := buildAWSWAFWebACLAssociation(cfg.StageName)
			template.Resources[WAFAssociationResourceName] = webACLAssociation
		}
	}

	template.Outputs = map[string]interface{}{
		OutputKeyRestAPIID:          Output{Value: cfn.Ref(APIResourceName)},
		OutputKeyAPIGatewayEndpoint: Output{Value: cfn.Join("", []string{"https://", cfn.Ref(APIResourceName), ".execute-api.", cfn.Ref("AWS::Region"), ".amazonaws.com/", cfg.StageName})},
		OutputKeyAPIEndpointType:    Output{Value: cfg.APIEndpointType},
		OutputKeyRequestTimeout:     Output{Value: fmt.Sprintf("%d", cfg.RequestTimeout)},
	}

	if cfg.UsagePlans != nil && len(cfg.UsagePlans) > 0 {
		val, _ := json.Marshal(cfg.UsagePlans)
		template.Outputs[OutputKeyUsagePlans] = Output{Value: string(val)}
	} else {
		template.Outputs[OutputKeyClientARNS] = Output{Value: strings.Join(cfg.Arns, ",")}
	}

	if cfg.MinimumCompressionSize > 0 {
		template.Outputs[OutputMinimumCompressionSize] = Output{fmt.Sprintf("%d", cfg.MinimumCompressionSize)}
	}

	if cfg.WAFEnabled {
		template.Outputs[OutputKeyWAFEnabled] = Output{Value: fmt.Sprintf("%t", cfg.WAFEnabled)}
		template.Outputs[OutputKeyWAFRules] = Output{Value: cfg.WAFRulesJSON}
		template.Outputs[OutputKeyWAFScope] = Output{Value: cfg.WAFScope}
	}

	if cfg.WAFAssociation {
		template.Outputs[OutputKeyWAFAssociationCreated] = Output{Value: cfn.Ref(WAFAssociationResourceName)}
	}

	if cfg.APIEndpointType == "REGIONAL" && cfg.CustomDomainName != "" {
		template.Outputs[OutputKeyCertARN] = Output{Value: cfg.CertificateArn}
		template.Outputs[OutputKeyCustomDomain] = Output{Value: cfg.CustomDomainName}
		template.Outputs[OutputKeyCustomDomainHostName] = Output{Value: cfn.GetAtt(CustomDomainResourceName, RegionalDomainNameResourceName)}
		template.Outputs[OutputKeyCustomDomainHostedZoneID] = Output{Value: cfn.GetAtt(CustomDomainResourceName, RegionalHostedZoneIdResourceName)}
		template.Outputs[OutputKeyTLSPolicy] = Output{Value: cfg.TLSPolicy}
		template.Outputs[OutputKeyCustomDomainBasePath] = Output{Value: cfg.CustomDomainBasePath}
	}

	if cfg.APIEndpointType == "EDGE" && cfg.CustomDomainName != "" {
		template.Outputs[OutputKeyCertARN] = Output{Value: cfg.CertificateArn}
		template.Outputs[OutputKeyCustomDomain] = Output{Value: cfg.CustomDomainName}
		template.Outputs[OutputKeyCustomDomainHostName] = Output{Value: cfn.GetAtt(CustomDomainResourceName, DistributionDomainNameResourceName)}
		template.Outputs[OutputKeyCustomDomainHostedZoneID] = Output{Value: cfn.GetAtt(CustomDomainResourceName, DistributionHostedZoneIdResourceName)}
		template.Outputs[OutputKeyTLSPolicy] = Output{Value: cfg.TLSPolicy}
		template.Outputs[OutputKeyCustomDomainBasePath] = Output{Value: cfg.CustomDomainBasePath}
	}

	return template
}

func buildCustomDomainRoute53Record(domainName string, hostedZoneName string, dnsName string, hostedZoneID string) *route53.RecordSet {
	return &route53.RecordSet{
		Name:           domainName,
		HostedZoneName: hostedZoneName,
		Type:           "A",
		AliasTarget: &route53.RecordSet_AliasTarget{
			DNSName:      dnsName,
			HostedZoneId: hostedZoneID,
		},
	}
}

//Route53TemplateConfig is the structure of configuration used to provide data to build the cf template of route53
type Route53TemplateConfig struct {
	CustomDomainName         string
	CustomDomainHostName     string
	CustomDomainHostedZoneID string
	HostedZoneName           string
}

// BuildAPIGatewayRoute53Template generates the cloudformation template according to the config provided
func BuildAPIGatewayRoute53Template(cfg *Route53TemplateConfig) *cfn.Template {
	route53Template := cfn.NewTemplate()

	if cfg.HostedZoneName != "" {
		recordSet := buildCustomDomainRoute53Record(cfg.CustomDomainName, cfg.HostedZoneName, cfg.CustomDomainHostName, cfg.CustomDomainHostedZoneID)
		route53Template.Resources[Route53RecordResourceName] = recordSet
	}

	route53Template.Outputs = map[string]interface{}{
		OutputKeyCustomDomainHostName:     Output{Value: cfg.CustomDomainHostName},
		OutputKeyCustomDomainHostedZoneID: Output{Value: cfg.CustomDomainHostedZoneID},
		OutputKeyCustomDomain:             Output{Value: cfg.CustomDomainName},
		OutputKeyHostedZone:               Output{Value: cfg.HostedZoneName},
	}

	return route53Template
}
