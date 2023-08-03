package handler

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func convertError(o dhcpv4.OptionCode, v string, extra string) error {
	err := fmt.Errorf("the value provided for DHCP option %v (%v) is invalid: %v", o.Code(), o.String(), v)
	if extra != "" {
		err = fmt.Errorf("the value provided for DHCP option %v (%v) is invalid: %v: %v", o.Code(), o.String(), v, extra)
	}

	return err
}

func one() {
	fmt.Println(all)
}

type Options struct {
	Opt1  net.IPMask
	Opt2  time.Duration // in seconds
	Opt3  []net.IP
	Opt4  []net.IP
	Opt5  []net.IP
	Opt6  []net.IP
	Opt7  []net.IP
	Opt8  []net.IP
	Opt9  []net.IP
	Opt10 []net.IP
	Opt11 []net.IP
	Opt12 string
	Opt13 [2]uint16
	Opt14 string
	Opt15 string
	Opt16 net.IP
	Opt17 string
	Opt18 string
	Opt19 [1]byte // 0 or 1 // can i use a bool?
	Opt20 [1]byte // 0 or 1 // can i use a bool?
	Opt21 []opt21
	Opt22 [2]uint16     // The minimum value legal value is 576
	Opt23 [1]byte       // between 1 and 255
	Opt24 time.Duration // seconds
}

var all = map[uint8]opt{
	1:  {dhcpv4.OptionSubnetMask, net.IPMask{}, nil},
	2:  {dhcpv4.OptionTimeOffset, time.Duration(0), nil},
	3:  {dhcpv4.OptionRouter, []net.IP{}, ipsToOption},
	4:  {dhcpv4.OptionTimeServer, []net.IP{}, ipsToOption},
	5:  {dhcpv4.OptionNameServer, []net.IP{}, ipsToOption},
	6:  {dhcpv4.OptionDomainNameServer, []net.IP{}, ipsToOption},
	7:  {dhcpv4.OptionLogServer, []net.IP{}, ipsToOption},
	8:  {dhcpv4.OptionQuoteServer, []net.IP{}, ipsToOption},
	9:  {dhcpv4.OptionLPRServer, []net.IP{}, ipsToOption},
	10: {dhcpv4.OptionImpressServer, []net.IP{}, ipsToOption},
	11: {dhcpv4.OptionResourceLocationServer, []net.IP{}, ipsToOption},
	12: {dhcpv4.OptionHostName, "", stringToOption},
	13: {dhcpv4.OptionBootFileSize, [2]uint16{}, nil},
	14: {dhcpv4.OptionMeritDumpFile, "", stringToOption},
	15: {dhcpv4.OptionDomainName, "", stringToOption},
	16: {dhcpv4.OptionSwapServer, net.IP{}, ipToOption},
	17: {dhcpv4.OptionRootPath, "", stringToOption},
	18: {dhcpv4.OptionExtensionsPath, "", stringToOption},
	19: {dhcpv4.OptionIPForwarding, [1]byte{}, nil},
	20: {dhcpv4.OptionNonLocalSourceRouting, [1]byte{}, nil},
	21: {dhcpv4.OptionPolicyFilter, []opt21{}, nil},
	22: {dhcpv4.OptionMaximumDatagramAssemblySize, [2]uint16{}, nil},
	23: {dhcpv4.OptionDefaultIPTTL, [1]byte{}, nil},
	24: {dhcpv4.OptionPathMTUAgingTimeout, time.Duration(0), nil},
	25: {dhcpv4.OptionPathMTUPlateauTable, []uint16{}, nil},
	26: {dhcpv4.OptionInterfaceMTU, [2]uint16{}, nil},
	27: {dhcpv4.OptionAllSubnetsAreLocal, [1]byte{}, nil},
	28: {dhcpv4.OptionBroadcastAddress, net.IP{}, ipToOption},
	29: {dhcpv4.OptionPerformMaskDiscovery, [1]byte{}, nil},
	30: {dhcpv4.OptionMaskSupplier, [1]byte{}, nil},
	31: {dhcpv4.OptionPerformRouterDiscovery, [1]byte{}, nil},
	32: {dhcpv4.OptionRouterSolicitationAddress, net.IP{}, ipToOption},
	33: {dhcpv4.OptionStaticRoutingTable, []opt33{}, nil},
	34: {dhcpv4.OptionTrailerEncapsulation, [1]byte{}, nil},
	35: {dhcpv4.OptionArpCacheTimeout, time.Duration(0), nil},
	36: {dhcpv4.OptionEthernetEncapsulation, [1]byte{}, nil},
	37: {dhcpv4.OptionDefaulTCPTTL, [1]byte{}, nil},
	38: {dhcpv4.OptionTCPKeepaliveInterval, time.Duration(0), nil},
	39: {dhcpv4.OptionTCPKeepaliveGarbage, [1]byte{}, nil},
	40: {dhcpv4.OptionNetworkInformationServiceDomain, "", nil},
	41: {dhcpv4.OptionNetworkInformationServers, []net.IP{}, ipsToOption},
	42: {dhcpv4.OptionNTPServers, []net.IP{}, ipsToOption},
	43: {dhcpv4.OptionVendorSpecificInformation, []byte{}, nil},
	44: {dhcpv4.OptionNetBIOSOverTCPIPNameServer, []net.IP{}, ipsToOption},
	45: {dhcpv4.OptionNetBIOSOverTCPIPDatagramDistributionServer, []net.IP{}, ipsToOption},
	46: {dhcpv4.OptionNetBIOSOverTCPIPNodeType, [1]byte{}, nil},
	47: {dhcpv4.OptionNetBIOSOverTCPIPScope, "", stringToOption},
	48: {dhcpv4.OptionXWindowSystemFontServer, []net.IP{}, ipsToOption},
	49: {dhcpv4.OptionXWindowSystemDisplayManger, []net.IP{}, ipsToOption},
	50: {dhcpv4.OptionRequestedIPAddress, net.IP{}, ipToOption},
	51: {dhcpv4.OptionIPAddressLeaseTime, time.Duration(0), nil},
	52: {dhcpv4.OptionOptionOverload, [1]byte{}, nil},
	53: {dhcpv4.OptionDHCPMessageType, [1]byte{}, nil},
	54: {dhcpv4.OptionServerIdentifier, net.IP{}, ipToOption},
	55: {dhcpv4.OptionParameterRequestList, []byte{}, nil},
	56: {dhcpv4.OptionMessage, "", stringToOption},
	57: {dhcpv4.OptionMaximumDHCPMessageSize, [2]uint16{}, nil},
	58: {dhcpv4.OptionRenewTimeValue, time.Duration(0), nil},
	59: {dhcpv4.OptionRebindingTimeValue, time.Duration(0), nil},
	60: {dhcpv4.OptionClassIdentifier, "", stringToOption},
	61: {dhcpv4.OptionClientIdentifier, []byte{}, nil},
	62: {dhcpv4.OptionNetWareIPDomainName, "", stringToOption},
	63: {dhcpv4.OptionNetWareIPInformation, []byte{}, nil},
	64: {dhcpv4.OptionNetworkInformationServicePlusDomain, "", stringToOption},
	65: {dhcpv4.OptionNetworkInformationServicePlusServers, []net.IP{}, ipsToOption},
	66: {dhcpv4.OptionTFTPServerName, "", stringToOption},
	67: {dhcpv4.OptionBootfileName, "", stringToOption},
	68: {dhcpv4.OptionMobileIPHomeAgent, []net.IP{}, ipsToOption},
	69: {dhcpv4.OptionSimpleMailTransportProtocolServer, []net.IP{}, ipsToOption},
	70: {dhcpv4.OptionPostOfficeProtocolServer, []net.IP{}, ipsToOption},
	71: {dhcpv4.OptionNetworkNewsTransportProtocolServer, []net.IP{}, ipsToOption},
	72: {dhcpv4.OptionDefaultWorldWideWebServer, []net.IP{}, ipsToOption},
	73: {dhcpv4.OptionDefaultFingerServer, []net.IP{}, ipsToOption},
	74: {dhcpv4.OptionDefaultInternetRelayChatServer, []net.IP{}, ipsToOption},
	75: {dhcpv4.OptionStreetTalkServer, []net.IP{}, ipsToOption},
	76: {dhcpv4.OptionStreetTalkDirectoryAssistanceServer, []net.IP{}, ipsToOption},
	77: {dhcpv4.OptionUserClassInformation, []byte{}, nil},
	78: {dhcpv4.OptionSLPDirectoryAgent, []net.IP{}, ipsToOption},
	79: {dhcpv4.OptionSLPServiceScope, []byte{}, nil},
	80: {dhcpv4.OptionRapidCommit, [1]byte{}, nil},
	81: {dhcpv4.OptionFQDN, "", stringToOption},
	82: {dhcpv4.OptionRelayAgentInformation, []byte{}, nil},
	83: {dhcpv4.OptionInternetStorageNameService, []net.IP{}, ipsToOption},
	// 84
	85: {dhcpv4.OptionNDSServers, []net.IP{}, ipsToOption},
	86: {dhcpv4.OptionNDSTreeName, "", stringToOption},
	87: {dhcpv4.OptionNDSContext, "", stringToOption},
	88: {dhcpv4.OptionBCMCSControllerDomainNameList, []string{}, sliceToOption},
	89: {dhcpv4.OptionBCMCSControllerIPv4AddressList, []net.IP{}, ipsToOption},
	90: {dhcpv4.OptionAuthentication, []byte{}, nil},
	91: {dhcpv4.OptionClientLastTransactionTime, time.Time{}, nil},
	92: {dhcpv4.OptionAssociatedIP, []net.IP{}, ipsToOption},
	93: {dhcpv4.OptionClientSystemArchitectureType, [2]byte{}, nil},
	94: {dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{}, nil},
	95: {dhcpv4.OptionLDAP, []net.IP{}, ipsToOption},
	// 96
	97:  {dhcpv4.OptionClientMachineIdentifier, []byte{}, nil},
	98:  {dhcpv4.OptionOpenGroupUserAuthentication, []byte{}, nil},
	99:  {dhcpv4.OptionGeoConfCivic, []byte{}, nil},
	100: {dhcpv4.OptionIEEE10031TZString, "", stringToOption},
	101: {dhcpv4.OptionReferenceToTZDatabase, "", stringToOption},
	// 102-111
	112: {dhcpv4.OptionNetInfoParentServerAddress, []net.IP{}, ipsToOption},
	113: {dhcpv4.OptionNetInfoParentServerTag, []byte{}, nil},
	114: {dhcpv4.OptionURL, "", stringToOption},
	// 115
	116: {dhcpv4.OptionAutoConfigure, []byte{}, nil},
	117: {dhcpv4.OptionNameServiceSearch, []byte{}, nil},
	118: {dhcpv4.OptionSubnetSelection, net.IP{}, ipToOption},
	119: {dhcpv4.OptionDNSDomainSearchList, []string{}, sliceToOption},
	120: {dhcpv4.OptionSIPServers, []net.IP{}, ipsToOption},
	121: {dhcpv4.OptionClasslessStaticRoute, []byte{}, nil},
	122: {dhcpv4.OptionCCC, []byte{}, nil},
	123: {dhcpv4.OptionGeoConf, []byte{}, nil},
	124: {dhcpv4.OptionVendorIdentifyingVendorClass, []byte{}, nil},
	125: {dhcpv4.OptionVendorIdentifyingVendorSpecific, []byte{}, nil},
	// 126-127
	128: {dhcpv4.OptionTFTPServerIPAddress, "", stringToOption},
	129: {dhcpv4.OptionCallServerIPAddress, "", stringToOption},
	130: {dhcpv4.OptionDiscriminationString, "", stringToOption},
	131: {dhcpv4.OptionRemoteStatisticsServerIPAddress, "", stringToOption},
	132: {dhcpv4.Option8021PVLANID, [2]byte{}, nil},
	133: {dhcpv4.Option8021QL2Priority, [2]byte{}, nil},
	134: {dhcpv4.OptionDiffservCodePoint, [1]byte{}, nil},
	135: {dhcpv4.OptionHTTPProxyForPhoneSpecificApplications, []net.IP{}, ipsToOption},
	136: {dhcpv4.OptionPANAAuthenticationAgent, []net.IP{}, ipsToOption},
	137: {dhcpv4.OptionLoSTServer, []net.IP{}, ipsToOption},
	138: {dhcpv4.OptionCAPWAPAccessControllerAddresses, []net.IP{}, ipsToOption},
	139: {dhcpv4.OptionOPTIONIPv4AddressMoS, []string{}, sliceToOption},
	140: {dhcpv4.OptionSIPUAConfigurationServiceDomains, []string{}, sliceToOption},
	141: {dhcpv4.OptionOPTIONIPv4FQDNMoS, []string{}, sliceToOption},
	142: {dhcpv4.OptionOPTIONIPv4AddressANDSF, []net.IP{}, ipsToOption},
	143: {dhcpv4.OptionOPTIONIPv6AddressANDSF, []byte{}, nil},
	// 144-149
	150: {dhcpv4.OptionTFTPServerAddress, []net.IP{}, ipsToOption},
	151: {dhcpv4.OptionStatusCode, []net.IP{}, ipsToOption},
	152: {dhcpv4.OptionBaseTime, []byte{}, nil},
	153: {dhcpv4.OptionStartTimeOfState, []net.IP{}, ipsToOption},
	154: {dhcpv4.OptionQueryStartTime, []string{}, sliceToOption},
	155: {dhcpv4.OptionQueryEndTime, []net.IP{}, ipsToOption},
	156: {dhcpv4.OptionDHCPState, []byte{}, byteToOption},
	157: {dhcpv4.OptionDataSource, []net.IP{}, ipsToOption},
	// 158-174
	175: {dhcpv4.OptionEtherboot, []byte{}, nil},
	176: {dhcpv4.OptionIPTelephone, []net.IP{}, ipsToOption},
	177: {dhcpv4.OptionEtherbootPacketCableAndCableHome, []byte{}, nil},
	// 178-207
	208: {dhcpv4.OptionPXELinuxMagicString, []byte{}, nil},
	209: {dhcpv4.OptionPXELinuxConfigFile, []byte{}, nil},
	210: {dhcpv4.OptionPXELinuxPathPrefix, []byte{}, nil},
	211: {dhcpv4.OptionPXELinuxRebootTime, []byte{}, nil},
	212: {dhcpv4.OptionOPTION6RD, []byte{}, nil},
	213: {dhcpv4.OptionOPTIONv4AccessDomain, []byte{}, nil},
	// 214-219
	220: {dhcpv4.OptionSubnetAllocation, []byte{}, nil},
	221: {dhcpv4.OptionVirtualSubnetAllocation, []byte{}, nil},
	// 222-254
	255: {dhcpv4.OptionEnd, nil, nil},
}

type opt struct {
	code       dhcpv4.OptionCode
	value      interface{}
	optionFunc func(dhcpv4.OptionCode, string) (dhcpv4.Option, error)
}

type opt21 struct {
	Address net.IP
	Mask    net.IPMask
}

type opt33 struct {
	Address net.IP
	Mask    net.IPMask
	Router  net.IP
}

func toIPS(s []string) ([]net.IP, error) {
	var ips []net.IP
	for _, ip := range s {
		if elem := net.ParseIP(ip); elem != nil {
			ips = append(ips, elem)
		} else {
			return nil, errors.New("invalid IP address: " + ip)
		}
	}
	return ips, nil
}

// ipsToOption converts a comma-separated string of IP addresses to a DHCP option.
func ipsToOption(opt dhcpv4.OptionCode, ips string) (dhcpv4.Option, error) {
	if i, err := toIPS(strings.Split(ips, ",")); err == nil {
		return dhcpv4.Option{
			Code:  opt,
			Value: dhcpv4.IPs(i),
		}, nil
	}

	return dhcpv4.Option{}, errors.New("invalid IP address: " + ips)
}

func ipToOption(opt dhcpv4.OptionCode, ip string) (dhcpv4.Option, error) {
	if elem := net.ParseIP(ip); elem != nil {
		return dhcpv4.Option{
			Code:  opt,
			Value: dhcpv4.IP(elem),
		}, nil
	}

	return dhcpv4.Option{}, errors.New("invalid IP address: " + ip)
}

func stringToOption(opt dhcpv4.OptionCode, s string) (dhcpv4.Option, error) {
	return dhcpv4.Option{
		Code:  opt,
		Value: dhcpv4.String(s),
	}, nil
}

func sliceToOption(opt dhcpv4.OptionCode, s string) (dhcpv4.Option, error) {
	return dhcpv4.Option{
		Code:  opt,
		Value: dhcpv4.Strings(strings.Split(s, ",")),
	}, nil
}

func byteToOption(opt dhcpv4.OptionCode, s string) (dhcpv4.Option, error) {
	i, err := strconv.Atoi(s)
	if err != nil {
		return dhcpv4.Option{}, err
	}
	// TODO: validation should not happen here. potentially need to validate each option differently
	if i < 1 || i > 7 {
		return dhcpv4.Option{}, errors.New("invalid : " + s)
	}
	o := []byte{byte(i)}
	if len(o) > 8 {
		return dhcpv4.Option{}, errors.New("invalid byte: " + s)
	}

	return dhcpv4.OptGeneric(opt, o), nil

}

func Convert(k uint8, v string) (dhcpv4.Option, error) {
	i := all[k]
	fmt.Println(k, v)
	return i.optionFunc(i.code, v)
	/*
	   switch i.code.Code() {
	   case 3, 4, 5, 6, 7, 8, 9, 10, 11:

	   		return ipsToOption(i.code, v)
	   	}

	   	m := map[uint8]func(string) (dhcpv4.Option, error){
	   		1: func(s string) (dhcpv4.Option, error) {
	   			if elem := net.ParseIP(s); elem != nil {
	   				return dhcpv4.OptSubnetMask(net.IPMask(elem.To4())), nil
	   			} else {
	   				return dhcpv4.Option{}, convertError(dhcpv4.OptionSubnetMask, s, "should be a subnet mask")
	   			}
	   		},
	   		2: func(s string) (dhcpv4.Option, error) {
	   			// validate that the string is an int64, add "s" to the end for seconds, and parse it as a duration
	   			if _, err := strconv.ParseInt(s, 10, 64); err != nil {
	   				return dhcpv4.Option{}, convertError(dhcpv4.OptionTimeOffset, s, "should be an integer string")
	   			}
	   			if elem, err := time.ParseDuration(s + "s"); err == nil {
	   				return dhcpv4.OptGeneric(dhcpv4.OptionTimeOffset, dhcpv4.Duration(elem).ToBytes()), nil
	   			} else {
	   				return dhcpv4.Option{}, convertError(dhcpv4.OptionTimeOffset, s, "should be an integer string")
	   			}
	   		},
	   		3: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionRouter)
	   		},
	   		4: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionTimeServer)
	   		},
	   		5: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionNameServer)
	   		},
	   		6: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionDomainNameServer)
	   		},
	   		7: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionLogServer)
	   		},
	   		8: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionQuoteServer)
	   		},
	   		9: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionLPRServer)
	   		},
	   		10: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionImpressServer)
	   		},
	   		11: func(s string) (dhcpv4.Option, error) {
	   			return ipsToOption(s, dhcpv4.OptionResourceLocationServer)
	   		},
	   		54: func(s string) (dhcpv4.Option, error) {
	   			if elem := net.ParseIP(s); elem != nil {
	   				return dhcpv4.OptServerIdentifier(net.ParseIP(s)), nil
	   			} else {
	   				return dhcpv4.Option{}, convertError(dhcpv4.OptionServerIdentifier, s, "should be an IP address")
	   			}
	   		},
	   	}

	   fn, found := m[k]

	   	if found {
	   		return fn(v)
	   	}

	   return dhcpv4.Option{}, fmt.Errorf("not found")
	*/
}
