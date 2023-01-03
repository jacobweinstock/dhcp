# Handler

## Reservation Handler

1. receive DHCP packet

- if option82 access control is enabled

    1. If the received DHCP packet DOES NOT pass any optional option82 access control rules, don't send a response, return immediately
    2. If the received DHCP packet passes any optional option82 access control rules, add/mirror the existing option 82 to the reply packet

- end if

2. query backend for record based on MAC address from the DHCP packet
3. If the MAC address is not found, don't send a response, return immediately
4. If the MAC address is found, populate a response packet with "general" DHCP options

- if netbooting is enabled && the backend record ALLOWS netbooting && the received DHCP packet identifies as a netboot client

    1. add netboot options to the response packet

- end if

- may not include this

    1. If the received DHCP packet requests a unicast response, send the response packet as a unicast packet (this includes when option82 is set) to the IP addr from the backend record.
    2. If the received DHCP packet does not request a unicast response, send the response packet as a broadcast packet

- may not include this

5. Send the response packet


### Netboot Options

Netboot options are different if the DHCP server is also providing "general" DHCP options versus if the DHCP server is not providing "general" DHCP options (i.e. just acting as a proxyDHCP server).

#### Netboot options for a general DHCP server

#### Netboot options for a proxyDHCP server

## When to unicast or broadcast

"If the 'giaddr' field in a DHCP message from a client is non-zero,
the server sends any return messages to the 'DHCP server' port on the
BOOTP relay agent whose address appears in 'giaddr'. If the 'giaddr'
field is zero and the 'ciaddr' field is nonzero, then the server
unicasts DHCPOFFER and DHCPACK messages to the address in 'ciaddr'.
If 'giaddr' is zero and 'ciaddr' is zero, and the broadcast bit is
set, then the server broadcasts DHCPOFFER and DHCPACK messages to
0xffffffff. If the broadcast bit is not set and 'giaddr' is zero and
'ciaddr' is zero, then the server unicasts DHCPOFFER and DHCPACK
messages to the client's hardware address and 'yiaddr' address."  -- https://www.ietf.org/rfc/rfc2131.txt

"1. If giaddr is 0 and ciaddr is non zero, the server sends a unicast message to address mentioned in ciaddr.
2. If giaddr is 0 and ciaddr is also 0, then server broadcast the message" -- https://iponwire.com/dhcp-header-explained/