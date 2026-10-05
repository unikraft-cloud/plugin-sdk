import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";
import "@unikraft/cloud-typespec-common";

using TypeSpec.Http;
using TypeSpec.OpenAPI;
using UnikraftCloud.Common;

@service(#{ title: "Unikraft Cloud Example Plugin API" })
@info(#{ version: "v1.0.0" })

namespace UnikraftCloud.Plugin.Example.V1;

// Types from the shared namespace would otherwise be emitted as
// `UnikraftCloud.Common.<X>` schemas, whose dotted names are not valid Go
// identifiers.  TODO: move these back into `@unikraft/cloud-typespec-common`.
@@friendlyName(UnikraftCloud.Common.ResponseStatus, "ResponseStatus");
@@friendlyName(UnikraftCloud.Common.ResponseError, "ResponseError");

model ExampleResponseData {
	message: string;
}

/** Response containing greeting details. */
@defaultResponse
model ExampleResponse is ResponseEnvelope<ExampleResponseData>;

/** Example Plugin API. */
@route("/")
@tag("Example")
interface ExampleApi {
	/** Greet the world. */
	@get
	@operationId("Greet")
	Greet(): ExampleResponse;

	/** Greet someone by name. */
	@get
	@route("/{who}")
	@operationId("GreetWho")
	GreetWho(
		@path who: string
	): ExampleResponse;
}
