# Automating Record Management with Reg-RWS

Source: https://www.arin.net/resources/registry/regrws/

Retrieved: 2026-09-22T22:50:50.892096+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## About Reg-RWS

Representational State Transfer (REST) is a type of client-server architecture that makes defining, addressing, and exchanging data easier. RESTful systems usually utilize URLs that are sent using an Application Programming Interface (API).

Registration RESTful Service (Reg-RWS) is a secure and efficient method for interacting with ARIN’s database and managing your registration records. Reg-RWS is most handy for repetitive, mundane tasks done in high volume with no needed human communication, such as reporting reassignments using the Shared Whois Project (SWIP).

## Reg-RWS Documentation

Generally, most users of ARIN’s Reg-RWS are engineers who code software that works with the Reg-RWS API to automate large numbers of transactions with the ARIN database. Although Reg-RWS commands can be entered individually into a browser, most users who want to run simple queries or report a few transactions use easier methods such as ARIN Online to do so.

Using Reg-RWS to manage registration records requires understanding of ARIN’s database structure, coding skills, and API programming knowledge. ARIN provides the following Reg-RWS documentation for users:

* [Quick Start Guide](https://www.arin.net/resources/registry/regrws/quickstart/): Provides a brief overview of prerequisites, how to construct Reg-RWS commands, and some command examples.
* [Reg-RWS Basics Explainer Video](https://youtu.be/Kn0GiLiVPfI): Walks through how to use the ARIN Reg-RWS API. Topics include API keys, Reg-RWS calls, and the anatomy of an API call.
* [RESTful Methods](https://www.arin.net/resources/registry/regrws/methods/): Lists supported methods (types of interactions with ARIN’s database) available in Reg-RWS.
* [RESTful Payloads](https://www.arin.net/resources/registry/regrws/payloads/): Lists the payloads used with methods in ARIN’s Reg-RWS. Payloads are XML or Routing Policy Specification Language (RPSL) files containing information that needs to be provided with some Reg-RWS methods, such as record modifications. (For example, if your method creates a new POC, the payload would contain the information for the POC such as name, address, contact type, etc.)

## Frequently Asked Questions about Reg-RWS

### How Does Reg-RWS Benefit Me?

REST is not intended for human consumption. It can be difficult to read, write, or understand, and is less than forgiving if you make an error. It is designed, rather, for computers and automation software. Computers can easily interact with REST for four main reasons:

* Predictable input and response mechanisms
* Discrete set of responses
* Instant responses
* Secure, authenticated communication

When working with Reg-RWS, you can program the computer so it knows what to say, when to say it, what the response will be, and how to interpret the response it receives, all with little to no additional human interference. What’s more, these instant transactions within ARIN’s database are secure and authenticated using a unique API Key.

### When Do I Benefit most from Reg-RWS?

Reg-RWS is great for many activities within ARIN’s database. It is secure and efficient for repetitive, mundane tasks done in high volume with no needed human communication. The most prominent example of this type of activity is SWIP: the process through which Internet Service Providers submit customer IP address space reassignment information to Whois.

### Why is SWIP Such a Good Fit for Reg-RWS?

Reg-RWS provides the instant, secure, predictable format for communicating with ARIN’s database that is ideally suited for SWIPs. Organizations and people who spend a great deal of time sending new customer reassignment data to ARIN’s database will find significant value in a system built to help automate these transactions.

### How Can I Test Reg-RWS Interactions Without Affecting My Production Data?

ARIN has implemented an Operational Test and Evaluation environment (OT&E) containing a copy of production-like data, refreshed monthly, that allows developers to experiment with ARIN interactions without affecting production data. For more information, visit the [OT&E page](https://www.arin.net/reference/tools/testing/).

## Troubleshooting

Errors happen for a variety of reasons. When Reg-RWS notifies you of an error with your URL/payload, it does so by returning an error code, along with a “body” containing an [Error Payload](https://www.arin.net/resources/registry/regrws/payloads/#error-payload) which will categorize the issue or issues contained within your submitted URL/payload.

If a call does not perform properly, there are several reasons why it may have failed. Be sure that:

* You have not filled in any immutable or system-generated fields

**Note:** When creating an object, all required fields not automatically generated must be filled out.

* You have not included any fields which do not belong in your payload
* You have not omitted a required field
* Your payload and URL are free of typographical errors
* Your URL, including API Key, is in the correct case (upper/lowercase)
* You selected the appropriate payload to send, and method with which to send it
* Your API Key is valid and linked to an ARIN Online account with the authority to create, update, or delete any records you are attempting to interact with
* Your URL contains the proper record identifiers (for example, the Org ID/Org Handle) and other required information per the method you are using
* Within your REST client, you have:
  + specified “US-ASCII or UTF-8” as your character set
  + specified the MIME type as “application/xml” or “application/rpsl” depending on your URL/payload

If the above list does not assist you in correcting the issue, please feel free to subscribe and submit a question to [ARIN’s tech-discuss mailing list,](http://lists.arin.net/mailman/listinfo/arin-tech-discuss) or by using the [Ask ARIN](https://account.arin.net/public/communication/message/beginQuestion.xhtml) feature within ARIN Online.

## Vendors

We are only aware of one vendor at this time who offers an IP Address Management (IPAM) product that is helpful for interfacing with ARIN’s Reg-RWS: 6connect ProVision with the [IPAM for IPv4 and IPv6 module](https://www.6connect.com/ipam/). If you are aware of other products, email [info@arin.net](mailto:info@arin.net) with details and we can add them.

ARIN also offers a set of [open source command-line scripts](https://github.com/arineng/arincli), written in Ruby, that utilize both Whois-RWS and the Reg-RWS (formerly ARINr).
