import React, { useState, useEffect, useContext } from "react";
import Moment from "react-moment";
import c from "classnames";

import api from "../../api.js";
import { appContext } from "../App";
import Chart from "../Chart";
import Duration from "../Duration";
import Website from "../Website";

import style from "./style.module.css";

const Entry = ({ entry }) => (
    <tr key={entry.time}>
        <td>
            <Moment date={entry.time} format="MMM DD, h:mma" />
        </td>
        <td>
            <span
                className={c(style.status, {
                    [style.bad]: entry.status !== "up"
                })}
            >
                {entry.status === "up" ? "Up" : "Down"}
            </span>
        </td>
        <td>
            <Duration value={entry.duration} />
        </td>
    </tr>
);

export default ({ params }) => {
    const { period } = useContext(appContext);

    const [website, setWebsite] = useState(null);
    const [checks, setChecks] = useState([]);
    const [history, setHistory] = useState([]);

    useEffect(() => {
        api.website(params.id).then(setWebsite);
        api.checks(params.id, ...period).then(setChecks);
        api.history(params.id, ...period).then(setHistory);
    }, [params.id]);

    if (!website) {
        return null;
    }

    return (
        <div className={style.history}>
            <header className={style.topbar}>
                <Website website={website} extended />
            </header>
            <div className={style.chart}>{checks.length ? <Chart checks={checks} /> : null}</div>
            <table className={style.table}>
                <tbody>
                    {history.map(entry => (
                        <Entry key={entry.time} entry={entry} />
                    ))}
                </tbody>
            </table>
        </div>
    );
};
