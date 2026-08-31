# Zoho CRM standard field recognition baseline

This catalog recognizes the approved standard field display names, API names, and data types for the 21 standard modules in [the module recognition baseline](zoho-crm-standard-modules.md). It contains the 609-field baseline only; it is not a write-safety, permission, layout, relationship, quota, or runtime-policy catalog.

Field identity is `(module_api_name, field_api_name)`. Field API name is authoritative. Display labels and data types are recognition hints, not operation-safety contracts. Runtime metadata remains authoritative for the target organization.

## Leads (`Leads`) — 43 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Change Log Time | `Change_Log_Time__s` | datetime |
| City | `City` | text |
| Company | `Company` | text |
| Converted Account | `Converted_Account` | lookup |
| Converted Contact | `Converted_Contact` | lookup |
| Converted Date Time | `Converted_Date_Time` | datetime |
| Converted Deal | `Converted_Deal` | lookup |
| Country | `Country` | text |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Email | `Email` | email |
| Email Opt Out | `Email_Opt_Out` | boolean |
| Enrich Status | `Enrich_Status__s` | picklist |
| Exchange Rate | `Exchange_Rate` | double |
| First Name | `First_Name` | text |
| Full Name | `Full_Name` | text |
| Is Converted | `Converted__s` | boolean |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Last Enriched Time | `Last_Enriched_Time__s` | datetime |
| Last Name | `Last_Name` | text |
| Lead Conversion Time | `Lead_Conversion_Time` | integer |
| Lead Image | `Record_Image` | profileimage |
| Lead Owner | `Owner` | ownerlookup |
| Lead Source | `Lead_Source` | picklist |
| Lead Status | `Lead_Status` | picklist |
| Locked | `Locked__s` | boolean |
| Mobile | `Mobile` | phone |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Phone | `Phone` | phone |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Salutation | `Salutation` | picklist |
| State | `State` | text |
| Street | `Street` | text |
| Tag | `Tag` | text |
| Title | `Designation` | text |
| Twitter | `Twitter` | text |
| Unsubscribed Mode | `Unsubscribed_Mode` | picklist |
| Unsubscribed Time | `Unsubscribed_Time` | datetime |
| Zip Code | `Zip_Code` | text |

## Accounts (`Accounts`) — 32 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Billing City | `Billing_City` | text |
| Billing Code | `Billing_Code` | text |
| Billing Country | `Billing_Country` | text |
| Billing State | `Billing_State` | text |
| Billing Street | `Billing_Street` | text |
| Change Log Time | `Change_Log_Time__s` | datetime |
| Client Image | `Record_Image` | profileimage |
| Client Name | `Account_Name` | text |
| Client Owner | `Owner` | ownerlookup |
| Client Type | `Account_Type` | picklist |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Enrich Status | `Enrich_Status__s` | picklist |
| Exchange Rate | `Exchange_Rate` | double |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Last Enriched Time | `Last_Enriched_Time__s` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Parent Client | `Parent_Account` | lookup |
| Phone | `Phone` | phone |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Shipping City | `Shipping_City` | text |
| Shipping Code | `Shipping_Code` | text |
| Shipping Country | `Shipping_Country` | text |
| Shipping State | `Shipping_State` | text |
| Shipping Street | `Shipping_Street` | text |
| Tag | `Tag` | text |
| Website | `Website` | website |

## Contacts (`Contacts`) — 42 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Change Log Time | `Change_Log_Time__s` | datetime |
| Client Name | `Account_Name` | lookup |
| Contact Image | `Record_Image` | profileimage |
| Contact Owner | `Owner` | ownerlookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Email | `Email` | email |
| Email Opt Out | `Email_Opt_Out` | boolean |
| Enrich Status | `Enrich_Status__s` | picklist |
| Exchange Rate | `Exchange_Rate` | double |
| First Name | `First_Name` | text |
| Full Name | `Full_Name` | text |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Last Enriched Time | `Last_Enriched_Time__s` | datetime |
| Last Name | `Last_Name` | text |
| Lead Source | `Lead_Source` | picklist |
| Locked | `Locked__s` | boolean |
| Mailing City | `Mailing_City` | text |
| Mailing Country | `Mailing_Country` | text |
| Mailing State | `Mailing_State` | text |
| Mailing Street | `Mailing_Street` | text |
| Mailing Zip | `Mailing_Zip` | text |
| Mobile | `Mobile` | phone |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Other City | `Other_City` | text |
| Other Country | `Other_Country` | text |
| Other State | `Other_State` | text |
| Other Street | `Other_Street` | text |
| Other Zip | `Other_Zip` | text |
| Phone | `Phone` | phone |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Reporting To | `Reporting_To` | lookup |
| Salutation | `Salutation` | picklist |
| Tag | `Tag` | text |
| Title | `Title` | text |
| Twitter | `Twitter` | text |
| Unsubscribed Mode | `Unsubscribed_Mode` | picklist |
| Unsubscribed Time | `Unsubscribed_Time` | datetime |

## Deals (`Deals`) — 28 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Amount | `Amount` | currency |
| Change Log Time | `Change_Log_Time__s` | datetime |
| Client Name | `Account_Name` | lookup |
| Closing Date | `Closing_Date` | date |
| Contact Name | `Contact_Name` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Exchange Rate | `Exchange_Rate` | double |
| Forecast Category | `Forecast_Category__s` | picklist |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Layout | `Layout` | layout |
| Lead Conversion Time | `Lead_Conversion_Time` | integer |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Opportunity / Project Name | `Deal_Name` | text |
| Opportunity / Project Owner | `Owner` | ownerlookup |
| Overall Sales Duration | `Overall_Sales_Duration` | integer |
| Pipeline | `Pipeline` | picklist |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Sales Cycle Duration | `Sales_Cycle_Duration` | integer |
| Stage | `Stage` | picklist |
| Stage Modified Time | `Stage_Modified_Time` | datetime |
| Tag | `Tag` | text |
| Type | `Type` | picklist |

## Campaigns (`Campaigns`) — 24 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Actual Cost | `Actual_Cost` | currency |
| Budgeted Cost | `Budgeted_Cost` | currency |
| Campaign Name | `Campaign_Name` | text |
| Campaign Owner | `Owner` | ownerlookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| End Date | `End_Date` | date |
| Exchange Rate | `Exchange_Rate` | double |
| Expected Response | `Expected_Response` | bigint |
| Expected Revenue | `Expected_Revenue` | currency |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Layout | `Layout` | layout |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Numbers sent | `Num_sent` | bigint |
| Parent Campaign | `Parent_Campaign` | lookup |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Start Date | `Start_Date` | date |
| Status | `Status` | picklist |
| Tag | `Tag` | text |
| Type | `Type` | picklist |

## Tasks (`Tasks`) — 23 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Closed Time | `Closed_Time` | datetime |
| Contact Name | `Who_Id` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Due Date | `Due_Date` | date |
| Exchange Rate | `Exchange_Rate` | double |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Priority | `Priority` | picklist |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Related To | `What_Id` | lookup |
| Reminder | `Remind_At` | ALARM |
| Repeat | `Recurring_Activity` | RRULE |
| Send Notification Email | `Send_Notification_Email` | boolean |
| Status | `Status` | picklist |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Task Owner | `Owner` | ownerlookup |

## Cases (`Cases`) — 31 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Add Comment | `Add_Comment` | textarea |
| Case Number | `Case_Number` | autonumber |
| Case Origin | `Case_Origin` | picklist |
| Case Owner | `Owner` | ownerlookup |
| Client Name | `Account_Name` | lookup |
| Comments | `Comments` | text |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Email | `Email` | email |
| Exchange Rate | `Exchange_Rate` | double |
| Internal Comments | `Internal_Comments` | textarea |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| No. of comments | `No_of_comments` | integer |
| Opportunity / Project Name | `Deal_Name` | lookup |
| Phone | `Phone` | phone |
| Priority | `Priority` | picklist |
| Product Name | `Product_Name` | lookup |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Related To | `Related_To` | lookup |
| Reported By | `Reported_By` | text |
| Solution | `Solution` | textarea |
| Status | `Status` | picklist |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Type | `Type` | picklist |

## Meetings (`Events`) — 37 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| All day | `All_day` | boolean |
| Checked In Status | `Check_In_Status` | text |
| Check-In Address | `Check_In_Address` | textarea |
| Check-In By | `Check_In_By` | ownerlookup |
| Check-In City | `Check_In_City` | text |
| Check-In Comment | `Check_In_Comment` | textarea |
| Check-In Country | `Check_In_Country` | text |
| Check-In State | `Check_In_State` | text |
| Check-In Sub-Locality | `Check_In_Sub_Locality` | text |
| Check-In Time | `Check_In_Time` | datetime |
| Contact Name | `Who_Id` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Exchange Rate | `Exchange_Rate` | double |
| From | `Start_DateTime` | datetime |
| Host | `Owner` | ownerlookup |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Latitude | `Latitude` | double |
| Location | `Venue` | text |
| Longitude | `Longitude` | double |
| Meeting Venue | `Meeting_Venue__s` | picklist |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Participants | `Participants` | bigint |
| Participants Reminder | `Remind Participants` | multireminder |
| Provider | `Meeting_Provider__s` | picklist |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Related To | `What_Id` | lookup |
| Reminder | `Remind_At` | multireminder |
| Repeat | `Recurring_Activity` | RRULE |
| Tag | `Tag` | text |
| Title | `Event_Title` | text |
| To | `End_DateTime` | datetime |
| Zip Code | `ZIP_Code` | text |

## Calls (`Calls`) — 27 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Call Agenda | `Call_Agenda` | text |
| Call Duration | `Call_Duration` | text |
| Call Duration (in seconds) | `Call_Duration_in_seconds` | integer |
| Call Owner | `Owner` | ownerlookup |
| Call Purpose | `Call_Purpose` | picklist |
| Call Result | `Call_Result` | text |
| Call Start Time | `Call_Start_Time` | datetime |
| Call Type | `Call_Type` | picklist |
| Caller ID | `Caller_ID` | text |
| Contact Name | `Who_Id` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| CTI Entry | `CTI_Entry` | boolean |
| Description | `Description` | textarea |
| Dialled Number | `Dialled_Number` | text |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Outgoing Call Status | `Outgoing_Call_Status` | picklist |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Related To | `What_Id` | lookup |
| Reminder | `Reminder` | picklist |
| Scheduled in CRM | `Scheduled_In_CRM` | picklist |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Voice Recording | `Voice_Recording__s` | website |

## Solutions (`Solutions`) — 22 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Add Comment | `Add_Comment` | textarea |
| Answer | `Answer` | textarea |
| Comments | `Comments` | text |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Exchange Rate | `Exchange_Rate` | double |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| No. of comments | `No_of_comments` | integer |
| Product Name | `Product_Name` | lookup |
| Published | `Published` | boolean |
| Question | `Question` | textarea |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Solution Number | `Solution_Number` | autonumber |
| Solution Owner | `Owner` | ownerlookup |
| Solution Title | `Solution_Title` | text |
| Status | `Status` | picklist |
| Tag | `Tag` | text |

## Products (`Products`) — 32 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Commission Rate | `Commission_Rate` | currency |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Description | `Description` | textarea |
| Handler | `Handler` | ownerlookup |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Manufacturer | `Manufacturer` | picklist |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Product Active | `Product_Active` | boolean |
| Product Category | `Product_Category` | picklist |
| Product Code | `Product_Code` | text |
| Product Image | `Record_Image` | profileimage |
| Product Name | `Product_Name` | text |
| Product Owner | `Owner` | ownerlookup |
| Qty Ordered | `Qty_Ordered` | double |
| Quantity in Demand | `Qty_in_Demand` | double |
| Quantity in Stock | `Qty_in_Stock` | double |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Reorder Level | `Reorder_Level` | double |
| Sales End Date | `Sales_End_Date` | date |
| Sales Start Date | `Sales_Start_Date` | date |
| Support End Date | `Support_Expiry_Date` | date |
| Support Start Date | `Support_Start_Date` | date |
| Tag | `Tag` | text |
| Tax | `Tax` | multiselectpicklist |
| Taxable | `Taxable` | boolean |
| Unit Price | `Unit_Price` | currency |
| Usage Unit | `Usage_Unit` | picklist |
| Vendor Name | `Vendor_Name` | lookup |

## Vendors (`Vendors`) — 27 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Category | `Category` | text |
| City | `City` | text |
| Country | `Country` | text |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Email | `Email` | email |
| Email Opt Out | `Email_Opt_Out` | boolean |
| Exchange Rate | `Exchange_Rate` | double |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Phone | `Phone` | phone |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| State | `State` | text |
| Street | `Street` | text |
| Tag | `Tag` | text |
| Unsubscribed Mode | `Unsubscribed_Mode` | picklist |
| Unsubscribed Time | `Unsubscribed_Time` | datetime |
| Vendor Image | `Record_Image` | profileimage |
| Vendor Name | `Vendor_Name` | text |
| Vendor Owner | `Owner` | ownerlookup |
| Website | `Website` | website |
| Zip Code | `Zip_Code` | text |

## Price Books (`Price_Books`) — 15 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Active | `Active` | boolean |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Description | `Description` | textarea |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Price Book Name | `Price_Book_Name` | text |
| Price Book Owner | `Owner` | ownerlookup |
| Pricing Details | `Pricing_Details` | text |
| Pricing Model | `Pricing_Model` | picklist |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Tag | `Tag` | text |

## Quotes (`Quotes`) — 38 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Adjustment | `Adjustment` | currency |
| Billing City | `Billing_City` | text |
| Billing Code | `Billing_Code` | text |
| Billing Country | `Billing_Country` | text |
| Billing State | `Billing_State` | text |
| Billing Street | `Billing_Street` | text |
| Carrier | `Carrier` | picklist |
| Client Name | `Account_Name` | lookup |
| Contact Name | `Contact_Name` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Discount | `Discount` | currency |
| Exchange Rate | `Exchange_Rate` | double |
| Grand Total | `Grand_Total` | formula |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Opportunity / Project Name | `Deal_Name` | lookup |
| Quote Number | `Quote_Number` | autonumber |
| Quote Owner | `Owner` | ownerlookup |
| Quote Stage | `Quote_Stage` | picklist |
| Quoted Items | `Quoted_Items` | subform |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Shipping City | `Shipping_City` | text |
| Shipping Code | `Shipping_Code` | text |
| Shipping Country | `Shipping_Country` | text |
| Shipping State | `Shipping_State` | text |
| Shipping Street | `Shipping_Street` | text |
| Sub Total | `Sub_Total` | formula |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Tax | `Tax` | currency |
| Terms and Conditions | `Terms_and_Conditions` | textarea |
| Valid Until | `Valid_Till` | date |

## Sales Orders (`Sales_Orders`) — 28 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Adjustment | `Adjustment` | currency |
| Client Name | `Account_Name` | lookup |
| Contact Name | `Contact_Name` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Discount | `Discount` | currency |
| Due Date | `Due_Date` | date |
| Exchange Rate | `Exchange_Rate` | double |
| Excise Duty | `Excise_Duty` | currency |
| Grand Total | `Grand_Total` | formula |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Opportunity / Project Name | `Deal_Name` | lookup |
| Ordered_Items | `Ordered_Items` | subform |
| Pending | `Pending` | text |
| PreInvoice Owner | `Owner` | ownerlookup |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| SO Number | `SO_Number` | autonumber |
| Status | `Status` | picklist |
| Sub Total | `Sub_Total` | formula |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Tax | `Tax` | currency |

## Purchase Orders (`Purchase_Orders`) — 41 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Adjustment | `Adjustment` | currency |
| Billing City | `Billing_City` | text |
| Billing Code | `Billing_Code` | text |
| Billing Country | `Billing_Country` | text |
| Billing State | `Billing_State` | text |
| Billing Street | `Billing_Street` | text |
| Carrier | `Carrier` | picklist |
| Contact Name | `Contact_Name` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Discount | `Discount` | currency |
| Due Date | `Due_Date` | date |
| Exchange Rate | `Exchange_Rate` | double |
| Excise Duty | `Excise_Duty` | currency |
| Grand Total | `Grand_Total` | formula |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Layout | `Layout` | layout |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| PO Date | `PO_Date` | date |
| PO Number | `PO_Number` | text |
| Purchase Order Owner | `Owner` | ownerlookup |
| Purchase_Items | `Purchase_Items` | subform |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Sales Commission | `Sales_Commission` | currency |
| Shipping City | `Shipping_City` | text |
| Shipping Code | `Shipping_Code` | text |
| Shipping Country | `Shipping_Country` | text |
| Shipping State | `Shipping_State` | text |
| Shipping Street | `Shipping_Street` | text |
| Status | `Status` | picklist |
| Sub Total | `Sub_Total` | formula |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Tax | `Tax` | currency |
| Terms and Conditions | `Terms_and_Conditions` | textarea |
| Vendor Name | `Vendor_Name` | lookup |

## Invoices (`Invoices`) — 41 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Adjustment | `Adjustment` | currency |
| Billing City | `Billing_City` | text |
| Billing Code | `Billing_Code` | text |
| Billing Country | `Billing_Country` | text |
| Billing State | `Billing_State` | text |
| Billing Street | `Billing_Street` | text |
| Client Name | `Account_Name` | lookup |
| Contact Name | `Contact_Name` | lookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Currency | `Currency` | picklist |
| Description | `Description` | textarea |
| Discount | `Discount` | currency |
| Due Date | `Due_Date` | date |
| Exchange Rate | `Exchange_Rate` | double |
| Excise Duty | `Excise_Duty` | currency |
| Grand Total | `Grand_Total` | formula |
| Invoice Date | `Invoice_Date` | date |
| Invoice Number | `Invoice_Number` | autonumber |
| Invoice Owner | `Owner` | ownerlookup |
| Invoiced Items | `Invoiced_Items` | subform |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Locked | `Locked__s` | boolean |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Purchase Order | `Purchase_Order` | text |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Sales Commission | `Sales_Commission` | currency |
| Sales Order | `Sales_Order` | lookup |
| Shipping City | `Shipping_City` | text |
| Shipping Code | `Shipping_Code` | text |
| Shipping Country | `Shipping_Country` | text |
| Shipping State | `Shipping_State` | text |
| Shipping Street | `Shipping_Street` | text |
| Status | `Status` | picklist |
| Sub Total | `Sub_Total` | formula |
| Subject | `Subject` | text |
| Tag | `Tag` | text |
| Tax | `Tax` | currency |
| Terms and Conditions | `Terms_and_Conditions` | textarea |

## Appointments (`Appointments__s`) — 32 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Additional Information | `Additional_Information` | textarea |
| Address | `Address` | text |
| Appointment End Time | `Appointment_End_Time` | datetime |
| Appointment For | `Appointment_For` | multi_module_lookup |
| Appointment Name | `Appointment_Name` | text |
| Appointment Start Time | `Appointment_Start_Time` | datetime |
| Booking Form Name | `Appointments_Booking_Form_Name__s` | lookup |
| Cancellation Note | `Cancellation_Note` | textarea |
| Cancellation Reason | `Cancellation_Reason` | picklist |
| Cancelled By | `Cancelled_By` | ownerlookup |
| Cancelled Time | `Cancelled_Time` | datetime |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Duration | `Duration` | integer |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Location | `Location` | picklist |
| Member | `Owner` | ownerlookup |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| No of Times Rescheduled | `Reschedule_Count` | integer |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Reminder | `Remind_At` | multireminder |
| Reschedule Note | `Reschedule_Note` | textarea |
| Reschedule Reason | `Reschedule_Reason` | picklist |
| Rescheduled By | `Rescheduled_By` | ownerlookup |
| Rescheduled From | `Rescheduled_From` | datetime |
| Rescheduled Time | `Rescheduled_Time` | datetime |
| Rescheduled To | `Rescheduled_To` | datetime |
| Service Name | `Service_Name` | lookup |
| Status | `Status` | picklist |
| Tag | `Tag` | text |

## Services (`Services__s`) — 25 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Available Day(s) | `Availability_Type` | picklist |
| Available Time | `Available_Timings` | time_range |
| Choose Date(s) | `Available_Dates` | date |
| Choose Day(s) | `Available_Days` | multiselectpicklist |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Description | `Description` | textarea |
| Duration | `Duration` | integer |
| Ends On | `Available_Till` | date |
| Last Activity Time | `Last_Activity_Time` | datetime |
| Location | `Location` | picklist |
| Member(s) | `Members` | multiuserlookup |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Price | `Price` | currency |
| Record Id | `id` | bigint |
| Service Image | `Record_Image` | profileimage |
| Service Name | `Service_Name` | text |
| Service Owner | `Owner` | ownerlookup |
| Starts From | `Available_From` | date |
| Status | `Status` | picklist |
| Tag | `Tag` | text |
| Tax | `Tax` | multiselectpicklist |
| Unavailable From | `Unavailable_From` | datetime |
| Unavailable Till | `Unavailable_Till` | datetime |

## Notes (`Notes`) — 11 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Associated_Id | `Associated_Id__s` | bigint |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Note Content | `Note_Content` | text |
| Note Owner | `Owner` | ownerlookup |
| Note Title | `Note_Title` | text |
| Parent ID | `Parent_Id` | multi_module_lookup |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |

## Attachments (`Attachments`) — 10 fields

| Field Display Name | Field API Name | Data Type |
|---|---|---|
| Attachment Owner | `Owner` | ownerlookup |
| Created By | `Created_By` | ownerlookup |
| Created Time | `Created_Time` | datetime |
| File Name | `File_Name` | text |
| Modified By | `Modified_By` | ownerlookup |
| Modified Time | `Modified_Time` | datetime |
| Parent ID | `Parent_Id` | lookup |
| Record Id | `id` | bigint |
| Record Status | `Record_Status__s` | picklist |
| Size | `Size` | bigint |
