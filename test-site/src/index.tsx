import React from "react";
import ReactDOM from "react-dom";
import Child from "./component/Child/index";
// import {Button, Input} from "antd";

class Page extends React.Component {
	render() {
		return (
			<div className={"test"}>
				<div>Page</div>
				<Child />
				{/*<Button>click me</Button>*/}
				{/*<Input/>*/}
			</div>
		);
	}
}

ReactDOM.render(<Page />, document.getElementById("root"));

// import "antd/es/button/style/index.css";
