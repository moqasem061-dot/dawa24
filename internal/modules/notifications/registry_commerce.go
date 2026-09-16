package notifications

func init() {
	// Commerce & Orders
	registerEvent(EventDefinition{
		Key:                EventOrderPlaced,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.view",
		TitleAr:            "تم استلام طلبك {order_number}",
		TitleEn:            "Order Received {order_number}",
		BodyAr:             "تم تسجيل طلبك بنجاح بإجمالي {total_amount} ج.م وجارٍ مراجعته من قبل المورد.",
		BodyEn:             "Your order has been recorded with total {total_amount} EGP and is being reviewed.",
	})
	registerEvent(EventDefinition{
		Key:                EventOrderStatusChanged,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.view",
		TitleAr:            "تحديث حالة الطلب {order_number}",
		TitleEn:            "Order Status Update {order_number}",
		BodyAr:             "قام المورد {vendor_name} بتحديث حالة طلبك إلى: {status}. {reason}",
		BodyEn:             "Supplier {vendor_name} updated order status to: {status}. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventOrderCancelledByBuyer,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.order.view",
		TitleAr:            "إلغاء الطلب من قبل المشتري {order_number}",
		TitleEn:            "Order Cancelled by Buyer {order_number}",
		BodyAr:             "قام المشتري {customer_name} بإلغاء الطلب {order_number}. السبب: {reason}",
		BodyEn:             "Buyer {customer_name} cancelled order {order_number}. Reason: {reason}",
	})

	// Purchase Requests & RFQs
	registerEvent(EventDefinition{
		Key:                EventPurchaseRequestCreated,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.purchase_request.view",
		TitleAr:            "طلب تسعير جديد #{request_id}",
		TitleEn:            "New RFQ #{request_id}",
		BodyAr:             "أرسلت صيدلية {pharmacy_name} طلب تسعير لـ {item_count} أصناف.",
		BodyEn:             "Pharmacy {pharmacy_name} submitted an RFQ with {item_count} items.",
	})
	registerEvent(EventDefinition{
		Key:                EventPurchaseRequestResponded,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.purchase_request.view",
		TitleAr:            "تم الرد على طلب التسعير #{request_id}",
		TitleEn:            "RFQ Response Received #{request_id}",
		BodyAr:             "قدم المورد {vendor_name} عرض أسعار لطلب التسعير الخاص بك.",
		BodyEn:             "Supplier {vendor_name} submitted a price quote for your RFQ.",
	})
	registerEvent(EventDefinition{
		Key:                EventQuoteRequested,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.purchase_request.view",
		TitleAr:            "طلب عرض سعر جديد",
		TitleEn:            "New Quote Requested",
		BodyAr:             "طلبت صيدلية {customer_name} عرض سعر لمنتج {product_name} بكمية {quantity}.",
		BodyEn:             "Pharmacy {customer_name} requested a price quote for {product_name} (qty: {quantity}).",
	})
	registerEvent(EventDefinition{
		Key:                EventQuoteProvided,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.purchase_request.view",
		TitleAr:            "تم تقديم عرض سعر لمنتجك",
		TitleEn:            "Price Quote Provided",
		BodyAr:             "قدم المورد {vendor_name} عرض سعر لمنتج {product_name} بسعر {quote_price} ج.م.",
		BodyEn:             "Supplier {vendor_name} provided a quote for {product_name} at {quote_price} EGP.",
	})
	registerEvent(EventDefinition{
		Key:                EventQuoteDecision,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.purchase_request.view",
		TitleAr:            "قرار بشأن عرض السعر",
		TitleEn:            "Quote Decision",
		BodyAr:             "قام العميل {customer_name} بـ {decision} عرض السعر الخاص بمنتج {product_name}.",
		BodyEn:             "Customer {customer_name} has {decision} the quote for {product_name}.",
	})
	registerEvent(EventDefinition{
		Key:                EventNegotiationOffer,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.order.negotiate",
		TitleAr:            "عرض تفاوض سعر جديد",
		TitleEn:            "New Negotiation Offer",
		BodyAr:             "اقترحت صيدلية {customer_name} سعراً تفاوضياً بقيمة {proposed_amount} ج.م للطلب {order_number}.",
		BodyEn:             "Pharmacy {customer_name} proposed {proposed_amount} EGP for order {order_number}.",
	})
	registerEvent(EventDefinition{
		Key:                EventNegotiationDecision,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.view",
		TitleAr:            "قرار التفاوض على السعر",
		TitleEn:            "Negotiation Decision",
		BodyAr:             "تم {decision} عرض التفاوض للطلب {order_number} من قبل المورد {vendor_name}. {reason}",
		BodyEn:             "Negotiation offer for order {order_number} was {decision} by {vendor_name}. {reason}",
	})

	// Promotions & Ads
	registerEvent(EventDefinition{
		Key:                EventSpecialOfferStatus,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.offer.view",
		TitleAr:            "حالة العرض الخاص: {status}",
		TitleEn:            "Special Offer Status: {status}",
		BodyAr:             "تم {status} العرض الخاص '{offer_title}'. {reason}",
		BodyEn:             "Special offer '{offer_title}' has been {status}. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventSponsorshipStatus,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.offer_package.view",
		TitleAr:            "حالة باقة الرعاية: {status}",
		TitleEn:            "Sponsorship Status: {status}",
		BodyAr:             "تم {status} طلب باقة الرعاية '{package_title}'. {reason}",
		BodyEn:             "Sponsorship package '{package_title}' has been {status}. {reason}",
	})
	registerEvent(EventDefinition{
		Key:                EventAdStatus,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "vendor.ad.view",
		TitleAr:            "حالة الإعلان: {status}",
		TitleEn:            "Advertisement Status: {status}",
		BodyAr:             "تم {status} إعلانك '{ad_title}'. {reason}",
		BodyEn:             "Your ad '{ad_title}' has been {status}. {reason}",
	})

	// Smart Order & Imports
	registerEvent(EventDefinition{
		Key:                EventSmartOrderRunFinished,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.create",
		TitleAr:            "اكتملت جولة الطلب الذكي بنجاح",
		TitleEn:            "Smart Order Run Completed",
		BodyAr:             "تمت معالجة جولة الطلب الذكي #{run_id} بنجاح. يمكنك الآن مراجعة المسودة وتأكيد الطلبات.",
		BodyEn:             "Smart order run #{run_id} completed successfully. You may now review and confirm orders.",
	})
	registerEvent(EventDefinition{
		Key:                EventSmartOrderRunFailed,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.create",
		TitleAr:            "تعذر إكمال جولة الطلب الذكي",
		TitleEn:            "Smart Order Run Failed",
		BodyAr:             "فشلت معالجة جولة الطلب الذكي #{run_id}: {error}. يرجى المحاولة مرة أخرى.",
		BodyEn:             "Smart order run #{run_id} failed: {error}. Please try again.",
	})
	registerEvent(EventDefinition{
		Key:                EventImportRunFinished,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "catalog.product.create",
		TitleAr:            "اكتمل استيراد المنتجات بنجاح",
		TitleEn:            "Catalog Import Completed",
		BodyAr:             "تم إكمال ملف الاستيراد #{run_id} بنجاح بعد معالجة {rows_count} صفاً.",
		BodyEn:             "Import run #{run_id} completed successfully with {rows_count} rows processed.",
	})
	registerEvent(EventDefinition{
		Key:                EventImportRunFailed,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "catalog.product.create",
		TitleAr:            "فشل استيراد المنتجات",
		TitleEn:            "Catalog Import Failed",
		BodyAr:             "تعذر استكمال عملية الاستيراد #{run_id}: {error}.",
		BodyEn:             "Catalog import run #{run_id} could not be completed: {error}.",
	})

	// Quotas
	registerEvent(EventDefinition{
		Key:                EventQuotaExhausted,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.create",
		TitleAr:            "استنفاد الحصة التوريدية للفرع",
		TitleEn:            "Branch Quota Exhausted",
		BodyAr:             "تم استنفاد حصة الفرع {branch_name} لدى المورد {vendor_name} للمنتج {product_name}.",
		BodyEn:             "Quota exhausted for branch {branch_name} with supplier {vendor_name} for {product_name}.",
	})
	registerEvent(EventDefinition{
		Key:                EventQuotaReleased,
		DefaultChannels:    []Channel{ChannelInApp},
		RequiredPermission: "pharmacy.order.create",
		TitleAr:            "إتاحة وتجديد الحصة التوريدية",
		TitleEn:            "Quota Released",
		BodyAr:             "قام المورد {vendor_name} بإتاحة وتجديد الحصة للفرع {branch_name} للمنتج {product_name}.",
		BodyEn:             "Supplier {vendor_name} released quota for branch {branch_name} for product {product_name}.",
	})
}
